// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ingest

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// ExtractPDFText 从 PDF 二进制数据中提取正文文本，按页拼接（供 LongText 等复用）
func ExtractPDFText(data []byte) (string, error) {
	return extractPDFText(data)
}

// extractPDFText 内部实现，供本包 loader 调用
func extractPDFText(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	reader, err := model.NewPdfReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("打开 PDF failed: %w", err)
	}

	numPages, err := reader.GetNumPages()
	if err != nil {
		return "", fmt.Errorf("获取页数failed: %w", err)
	}
	if numPages == 0 {
		return "", nil
	}

	var buf strings.Builder
	for i := 1; i <= numPages; i++ {
		page, err := reader.GetPage(i)
		if err != nil {
			return buf.String(), fmt.Errorf("获取第 %d 页failed: %w", i, err)
		}
		ex, err := extractor.New(page)
		if err != nil {
			return buf.String(), fmt.Errorf("创建第 %d 页提取器failed: %w", i, err)
		}
		text, err := ex.ExtractText()
		if err != nil {
			return buf.String(), fmt.Errorf("提取第 %d 页文本failed: %w", i, err)
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
