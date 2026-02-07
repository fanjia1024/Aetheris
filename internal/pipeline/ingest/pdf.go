package ingest

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// extractPDFText 从 PDF 二进制数据中提取正文文本，按页拼接。
func extractPDFText(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	reader, err := model.NewPdfReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("打开 PDF 失败: %w", err)
	}

	numPages, err := reader.GetNumPages()
	if err != nil {
		return "", fmt.Errorf("获取页数失败: %w", err)
	}
	if numPages == 0 {
		return "", nil
	}

	var buf strings.Builder
	for i := 1; i <= numPages; i++ {
		page, err := reader.GetPage(i)
		if err != nil {
			return buf.String(), fmt.Errorf("获取第 %d 页失败: %w", i, err)
		}
		ex, err := extractor.New(page)
		if err != nil {
			return buf.String(), fmt.Errorf("创建第 %d 页提取器失败: %w", i, err)
		}
		text, err := ex.ExtractText()
		if err != nil {
			return buf.String(), fmt.Errorf("提取第 %d 页文本失败: %w", i, err)
		}
		if text != "" {
			buf.WriteString(text)
			if i < numPages {
				buf.WriteString("\n\n")
			}
		}
	}

	return strings.TrimSpace(buf.String()), nil
}
