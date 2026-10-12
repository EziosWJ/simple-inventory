package exportfile

import (
	"errors"
	"strings"
	"time"
)

const RowLimit = 100000

var ErrLimit = errors.New("匹配行数超过导出上限（100000 行），请缩小查询范围")

// BusinessTime agrees with the UI's Asia/Shanghai calendar and has no host
// timezone dependency. Modern business timestamps use UTC+08:00.
func BusinessTime(t time.Time) time.Time { return t.In(time.FixedZone("Asia/Shanghai", 8*60*60)) }
func ExportDateRange(from, to string, now time.Time) string {
	if from == "" && to == "" {
		return "全部_" + BusinessTime(now).Format("20060102")
	}
	start, end := strings.ReplaceAll(from, "-", ""), strings.ReplaceAll(to, "-", "")
	if start == "" {
		start = "全部"
	}
	if end == "" {
		end = "全部"
	}
	rangeText := start + "-" + end
	if from == "" || to == "" {
		rangeText += "_" + BusinessTime(now).Format("20060102")
	}
	return rangeText
}
