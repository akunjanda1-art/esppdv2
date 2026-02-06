package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"esppd.local/document-service/internal/repo"
	"esppd.local/shared/cryptox"
	"esppd.local/shared/httpx"
	"esppd.local/shared/jwtx"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/nats-io/nats.go"
	"github.com/phpdave11/gofpdf"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	repo     *repo.Repo
	verifier *jwtx.Verifier
	aesgcm   *cryptox.AESGCM
	nc       *nats.Conn
	mc       *minio.Client
	bucket   string
}

func NewHandler(r *repo.Repo, v *jwtx.Verifier, aes *cryptox.AESGCM, nc *nats.Conn, mc *minio.Client, bucket string) *Handler {
	return &Handler{repo: r, verifier: v, aesgcm: aes, nc: nc, mc: mc, bucket: bucket}
}

func (h *Handler) RegisterRoutes(r fiber.Router) {
	g := r.Group("/documents", httpx.JWTAuth(h.verifier, true))
	g.Post("/spds/:spd_id/generate", h.Generate)
}

type generateRequest struct {
	Format string `json:"format"`
}

type job struct {
	JobID       string    `json:"job_id"`
	SPDID       int64     `json:"spd_id"`
	Format      string    `json:"format"`
	RequestedBy int64     `json:"requested_by"`
	Role        string    `json:"role"`
	RequestedAt time.Time `json:"requested_at"`
}

func (h *Handler) Generate(c *fiber.Ctx) error {
	spdID, err := httpx.ParseIDParam(c, "spd_id")
	if err != nil {
		return err
	}
	a, ok := httpx.AuthFromLocals(c)
	if !ok {
		return fiber.NewError(http.StatusUnauthorized, "unauthorized")
	}

	var req generateRequest
	_ = c.BodyParser(&req)
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "pdf"
	}
	if format != "pdf" && format != "docx" && format != "xlsx" {
		return fiber.NewError(http.StatusBadRequest, "format must be pdf|docx|xlsx")
	}

	jobID, err := newID(16)
	if err != nil {
		return err
	}
	j := job{JobID: jobID, SPDID: spdID, Format: format, RequestedBy: a.UserID, Role: a.Role, RequestedAt: time.Now().UTC()}

	// Async path (preferred)
	if h.nc != nil {
		b, _ := json.Marshal(j)
		if err := h.nc.Publish("document.generate", b); err != nil {
			return err
		}
		return c.Status(http.StatusAccepted).JSON(fiber.Map{"job_id": jobID, "status": "queued"})
	}

	// Sync fallback
	res, err := h.processJob(context.Background(), j)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(res)
}

func (h *Handler) StartNATSConsumers(ctx context.Context) {
	sub, err := h.nc.Subscribe("document.generate", func(m *nats.Msg) {
		var j job
		if err := json.Unmarshal(m.Data, &j); err != nil {
			log.Warn().Err(err).Msg("invalid document.generate event")
			return
		}
		if _, err := h.processJob(context.Background(), j); err != nil {
			log.Error().Err(err).Str("job_id", j.JobID).Msg("process document job")
		}
	})
	if err != nil {
		log.Warn().Err(err).Msg("subscribe document.generate")
		return
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()
}

type documentResult struct {
	DocumentID int64  `json:"document_id"`
	SPDID      int64  `json:"spd_id"`
	Format     string `json:"format"`
	ObjectKey  string `json:"object_key"`
	CreatedAt  string `json:"created_at"`
}

func (h *Handler) processJob(ctx context.Context, j job) (*documentResult, error) {
	spd, err := h.repo.GetSPDForDoc(ctx, j.RequestedBy, j.Role, j.SPDID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fiber.NewError(http.StatusNotFound, "spd not found")
		}
		return nil, err
	}

	purpose, err := h.aesgcm.DecryptString(spd.PurposeEnc, []byte("spds:purpose"))
	if err != nil {
		return nil, err
	}
	total, err := h.aesgcm.DecryptString(spd.TotalCostEnc, []byte("spds:total_cost"))
	if err != nil {
		return nil, err
	}

	content, contentType, ext, err := buildDocument(j.Format, spd.NomorSurat, purpose, total)
	if err != nil {
		return nil, err
	}

	objectKey := fmt.Sprintf("spds/%d/spd_%d_%s.%s", j.SPDID, j.SPDID, j.JobID, ext)
	reader := bytes.NewReader(content)
	_, err = h.mc.PutObject(ctx, h.bucket, objectKey, reader, int64(len(content)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return nil, err
	}

	doc, err := h.repo.InsertDocument(ctx, j.RequestedBy, j.Role, j.SPDID, j.Format, objectKey, j.RequestedBy)
	if err != nil {
		return nil, err
	}

	if h.nc != nil {
		_ = h.nc.Publish("document.generated", []byte(fmt.Sprintf(`{"document_id":%d,"spd_id":%d}`, doc.ID, doc.SPDID)))
	}

	return &documentResult{
		DocumentID: doc.ID,
		SPDID:      doc.SPDID,
		Format:     doc.Format,
		ObjectKey:  doc.ObjectKey,
		CreatedAt:  doc.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func buildDocument(format, nomorSurat, purpose, total string) ([]byte, string, string, error) {
	switch format {
	case "pdf":
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdf.SetTitle("SPD", false)
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(40, 10, "Surat Perintah Perjalanan Dinas")
		pdf.Ln(12)
		pdf.SetFont("Arial", "", 12)
		pdf.Cell(40, 8, "Nomor Surat: "+nomorSurat)
		pdf.Ln(8)
		pdf.MultiCell(0, 6, "Tujuan: "+purpose, "", "L", false)
		pdf.Ln(2)
		pdf.Cell(40, 8, "Total Biaya: "+total)
		var buf bytes.Buffer
		if err := pdf.Output(&buf); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), "application/pdf", "pdf", nil
	case "docx":
		b, err := buildDOCX(nomorSurat, purpose, total)
		if err != nil {
			return nil, "", "", err
		}
		return b, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", nil
	case "xlsx":
		b, err := buildXLSX(nomorSurat, purpose, total)
		if err != nil {
			return nil, "", "", err
		}
		return b, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx", nil
	default:
		return nil, "", "", fiber.NewError(http.StatusBadRequest, "unsupported format")
	}
}

func newID(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
