package http

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestBuildDOCX_MinimalZip(t *testing.T) {
	b, err := buildDOCX("SPD-<A&B>", "Tujuan & Rencana", "1000")
	if err != nil {
		t.Fatalf("buildDOCX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}

	mustHave := map[string]bool{
		"[Content_Types].xml": false,
		"_rels/.rels":         false,
		"word/document.xml":   false,
	}
	var doc string
	for _, f := range zr.File {
		if _, ok := mustHave[f.Name]; ok {
			mustHave[f.Name] = true
		}
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open document.xml: %v", err)
			}
			raw, _ := io.ReadAll(rc)
			_ = rc.Close()
			doc = string(raw)
		}
	}

	for k, ok := range mustHave {
		if !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if !strings.Contains(doc, "SPD-&lt;A&amp;B&gt;") {
		t.Fatalf("expected escaped nomor surat in document.xml; got: %s", doc)
	}
}

func TestBuildXLSX_MinimalZip(t *testing.T) {
	b, err := buildXLSX("SPD-001", "X & Y", "500")
	if err != nil {
		t.Fatalf("buildXLSX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}

	mustHave := map[string]bool{
		"[Content_Types].xml":        false,
		"_rels/.rels":                false,
		"xl/workbook.xml":            false,
		"xl/_rels/workbook.xml.rels": false,
		"xl/worksheets/sheet1.xml":   false,
	}
	var sheet string
	for _, f := range zr.File {
		if _, ok := mustHave[f.Name]; ok {
			mustHave[f.Name] = true
		}
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open sheet1.xml: %v", err)
			}
			raw, _ := io.ReadAll(rc)
			_ = rc.Close()
			sheet = string(raw)
		}
	}

	for k, ok := range mustHave {
		if !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if !strings.Contains(sheet, "<t>X &amp; Y</t>") {
		t.Fatalf("expected escaped purpose in sheet1.xml; got: %s", sheet)
	}
}
