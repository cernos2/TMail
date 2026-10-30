package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Spreadsheet preview built directly from the .xlsx (milliseconds instead of a LibreOffice export):
// one HTML table per sheet with merged cells, column widths, bold text and number formats.
// .xls / .ods are converted to .xlsx first by the preview container.

const (
	sheetMaxRows = 3000
	sheetMaxCols = 120
)

func (s *Server) sheetHTMLFast(ctx context.Context, name string, data []byte) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".xlsx" && ext != ".xlsm" {
		x, err := s.officeRenderQ(ctx, "convert", "&to=xlsx", name, data)
		if err != nil {
			return nil, err
		}
		data = x
	}
	// Limits against zip/XML bombs in a received spreadsheet.
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 200 << 20, UnzipXMLSizeLimit: 64 << 20})
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var b strings.Builder
	for _, sheet := range f.GetSheetList() {
		if v, _ := f.GetSheetVisible(sheet); !v {
			continue
		}
		rows, err := sheetRows(ctx, f, sheet)
		if err != nil {
			continue
		}
		cols := 0
		for _, r := range rows {
			cols = max(cols, len(r))
		}
		cols = min(cols, sheetMaxCols)
		// Merged cells: the top-left one gets col/rowspan, the others are skipped.
		span := map[[2]int][2]int{}
		hidden := map[[2]int]bool{}
		if merges, err := f.GetMergeCells(sheet); err == nil {
			for _, m := range merges {
				c1, r1, e1 := excelize.CellNameToCoordinates(m.GetStartAxis())
				c2, r2, e2 := excelize.CellNameToCoordinates(m.GetEndAxis())
				if e1 != nil || e2 != nil {
					continue
				}
				// A merge like A1:XFD1048576 would mean 10^10 cells: only the visible part matters.
				if r1 > sheetMaxRows || c1 > sheetMaxCols {
					continue
				}
				r2, c2 = min(r2, sheetMaxRows), min(c2, sheetMaxCols)
				span[[2]int{r1, c1}] = [2]int{c2 - c1 + 1, r2 - r1 + 1}
				for r := r1; r <= r2; r++ {
					for c := c1; c <= c2; c++ {
						if r != r1 || c != c1 {
							hidden[[2]int{r, c}] = true
						}
					}
				}
				cols = min(max(cols, c2), sheetMaxCols)
			}
		}
		fmt.Fprintf(&b, "<h1>%s</h1>\n<table cellspacing=\"0\" border=\"0\"><colgroup>", html.EscapeString(sheet))
		for c := 1; c <= cols; c++ {
			col, _ := excelize.ColumnNumberToName(c)
			w, _ := f.GetColWidth(sheet, col)
			fmt.Fprintf(&b, `<col style="width:%dpx">`, int(w*7+5))
		}
		b.WriteString("</colgroup>\n")
		for ri, r := range rows {
			b.WriteString("<tr>")
			for c := 1; c <= cols; c++ {
				if hidden[[2]int{ri + 1, c}] {
					continue
				}
				val := ""
				if c-1 < len(r) {
					val = r[c-1]
				}
				attrs, style := "", ""
				if sp, ok := span[[2]int{ri + 1, c}]; ok {
					if sp[0] > 1 {
						attrs += fmt.Sprintf(` colspan="%d"`, sp[0])
					}
					if sp[1] > 1 {
						attrs += fmt.Sprintf(` rowspan="%d"`, sp[1])
					}
				}
				cell, _ := excelize.CoordinatesToCellName(c, ri+1)
				if id, err := f.GetCellStyle(sheet, cell); err == nil && id > 0 {
					if st, err := f.GetStyle(id); err == nil && st != nil {
						if st.Font != nil && st.Font.Bold {
							style += "font-weight:bold;"
						}
						if st.Alignment != nil {
							switch st.Alignment.Horizontal {
							case "center", "centerContinuous":
								style += "text-align:center;"
							case "right":
								style += "text-align:right;"
							}
						}
						if len(st.Fill.Color) > 0 && st.Fill.Pattern == 1 {
							style += "background:#" + strings.TrimPrefix(st.Fill.Color[0], "#") + ";"
						}
					}
				}
				if style == "" && looksNumeric(val) {
					style = "text-align:right;"
				}
				if style != "" {
					attrs += ` style="` + style + `"`
				}
				b.WriteString("<td" + attrs + ">" + html.EscapeString(val) + "</td>")
			}
			b.WriteString("</tr>\n")
		}
		b.WriteString("</table>\n")
	}
	return []byte(sheetPolicy.Sanitize(b.String())), nil
}

func looksNumeric(v string) bool {
	// Spaces, percent and ruble signs (including the Russian abbreviations of "rubles") are stripped.
	v = strings.TrimSpace(strings.NewReplacer(" ", "", " ", "", ",", ".", "%", "", "₽", "", "р.", "", "руб.", "").Replace(v))
	if v == "" {
		return false
	}
	for i, r := range v {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-' && i == 0) {
			return false
		}
	}
	return true
}

// sheetRows streams at most sheetMaxRows rows instead of loading the whole sheet like GetRows.
func sheetRows(ctx context.Context, f *excelize.File, sheet string) ([][]string, error) {
	it, err := f.Rows(sheet)
	if err != nil {
		return nil, err
	}
	defer it.Close()
	var rows [][]string
	for len(rows) < sheetMaxRows && it.Next() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		r, err := it.Columns()
		if err != nil {
			return nil, err
		}
		if len(r) > sheetMaxCols {
			r = r[:sheetMaxCols]
		}
		rows = append(rows, r)
	}
	return rows, it.Error()
}

// sheetText: every visible sheet as tab-separated rows (for AI agents). Only .xlsx/.xlsm are read
// directly; other spreadsheet formats are not supported here.
func sheetText(ctx context.Context, name string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".xlsx" && ext != ".xlsm" {
		return "", fmt.Errorf("unsupported spreadsheet %s", ext)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 200 << 20, UnzipXMLSizeLimit: 64 << 20})
	if err != nil {
		return "", err
	}
	defer f.Close()
	var b strings.Builder
	for _, sheet := range f.GetSheetList() {
		if v, _ := f.GetSheetVisible(sheet); !v {
			continue
		}
		rows, err := sheetRows(ctx, f, sheet)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "## Sheet %q\n", sheet)
		for _, r := range rows {
			line := strings.TrimRight(strings.Join(r, "\t"), "\t")
			if line != "" {
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
