package http

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
)

func buildDOCX(nomorSurat, purpose, total string) ([]byte, error) {
	nomorSurat = xmlEscape(nomorSurat)
	purpose = xmlEscape(purpose)
	total = xmlEscape(total)

	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <w:body>
    <w:p><w:r><w:t>Surat Perintah Perjalanan Dinas</w:t></w:r></w:p>
    <w:p><w:r><w:t>Nomor Surat: %s</w:t></w:r></w:p>
    <w:p><w:r><w:t>Tujuan: %s</w:t></w:r></w:p>
    <w:p><w:r><w:t>Total Biaya: %s</w:t></w:r></w:p>
    <w:sectPr/>
  </w:body>
</w:document>`, nomorSurat, purpose, total)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zipAdd(zw, "[Content_Types].xml", []byte(contentTypes)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zipAdd(zw, "_rels/.rels", []byte(rels)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zipAdd(zw, "word/document.xml", []byte(docXML)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildXLSX(nomorSurat, purpose, total string) ([]byte, error) {
	nomorSurat = xmlEscape(nomorSurat)
	purpose = xmlEscape(purpose)
	total = xmlEscape(total)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

	workbook := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="SPD" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`

	workbookRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`

	sheet := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1">
      <c r="A1" t="inlineStr"><is><t>nomor_surat</t></is></c>
      <c r="B1" t="inlineStr"><is><t>purpose</t></is></c>
      <c r="C1" t="inlineStr"><is><t>total</t></is></c>
    </row>
    <row r="2">
      <c r="A2" t="inlineStr"><is><t>%s</t></is></c>
      <c r="B2" t="inlineStr"><is><t>%s</t></is></c>
      <c r="C2" t="inlineStr"><is><t>%s</t></is></c>
    </row>
  </sheetData>
</worksheet>`, nomorSurat, purpose, total)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := zipAdd(zw, "[Content_Types].xml", []byte(contentTypes)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zipAdd(zw, "_rels/.rels", []byte(rels)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zipAdd(zw, "xl/workbook.xml", []byte(workbook)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zipAdd(zw, "xl/_rels/workbook.xml.rels", []byte(workbookRels)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zipAdd(zw, "xl/worksheets/sheet1.xml", []byte(sheet)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func zipAdd(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
