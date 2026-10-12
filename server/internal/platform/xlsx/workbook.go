// Package xlsx writes workbooks from columns and rows supplied by callers.
package xlsx

import (
	"fmt"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// Number retains the decimal representation supplied by the domain without a
// floating point conversion. Money applies the two-decimal currency format.
type Number struct {
	Decimal string
	Money   bool
}

func Decimal(value string) Number { return Number{Decimal: value} }
func Amount(value string) Number  { return Number{Decimal: value, Money: true} }
func Optional(value *string, money bool) any {
	if value == nil {
		return nil
	}
	return Number{Decimal: *value, Money: money}
}

type Sheet struct {
	Name    string
	Headers []string
	Rows    [][]any
	// HeaderRows are additional one-based header rows, useful for summary sections.
	HeaderRows []int
}

func Build(sheets ...Sheet) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	format := "#,##0.00"
	money, err := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
	if err != nil {
		return nil, err
	}
	for index, sheet := range sheets {
		if index == 0 {
			err = f.SetSheetName("Sheet1", sheet.Name)
		} else {
			_, err = f.NewSheet(sheet.Name)
		}
		if err != nil {
			return nil, err
		}
		rows := make([][]any, 0, len(sheet.Rows)+1)
		if len(sheet.Headers) > 0 {
			header := make([]any, len(sheet.Headers))
			for i, v := range sheet.Headers {
				header[i] = v
			}
			rows = append(rows, header)
		}
		rows = append(rows, sheet.Rows...)
		widths := map[int]float64{}
		for r, row := range rows {
			for c, value := range row {
				if value == nil {
					continue
				}
				cell, err := excelize.CoordinatesToCellName(c+1, r+1)
				if err != nil {
					return nil, err
				}
				switch v := value.(type) {
				case *string:
					if v == nil {
						continue
					}
					value = *v
					err = f.SetCellStr(sheet.Name, cell, *v)
				case Number:
					err = f.SetCellDefault(sheet.Name, cell, v.Decimal)
					if err == nil && v.Money {
						err = f.SetCellStyle(sheet.Name, cell, cell, money)
					}
					value = v.Decimal
				case string:
					// All labels, identifiers and dates stay literal strings, including text
					// that starts with '='; it must never become an Excel formula.
					err = f.SetCellStr(sheet.Name, cell, v)
				default:
					err = f.SetCellValue(sheet.Name, cell, v)
				}
				if err != nil {
					return nil, err
				}
				width := float64(2)
				for _, ch := range fmt.Sprint(value) {
					if unicode.Is(unicode.Han, ch) {
						width += 2
					} else {
						width++
					}
				}
				if width > 60 {
					width = 60
				}
				if width > widths[c+1] {
					widths[c+1] = width
				}
			}
		}
		headerRows := append([]int{1}, sheet.HeaderRows...)
		for _, r := range headerRows {
			last := len(sheet.Headers)
			if last == 0 && r <= len(rows) {
				last = len(rows[r-1])
			}
			if last == 0 {
				continue
			}
			end, _ := excelize.CoordinatesToCellName(last, r)
			if err = f.SetCellStyle(sheet.Name, fmt.Sprintf("A%d", r), end, bold); err != nil {
				return nil, err
			}
		}
		for c, width := range widths {
			col, _ := excelize.ColumnNumberToName(c)
			if err = f.SetColWidth(sheet.Name, col, col, width); err != nil {
				return nil, err
			}
		}
		if err = f.SetPanes(sheet.Name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
			return nil, err
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
