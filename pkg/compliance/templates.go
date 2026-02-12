// Copyright 2026 fanjia1024
// Compliance templates (GDPR/SOX/HIPAA) (3.0-M4)

package compliance

import (
	"rag-platform/pkg/redaction"
)

// ComplianceTemplate 合规模板
type ComplianceTemplate struct {
	Name           string
	Standard       string
	RetentionDays  int
	RedactionRules []redaction.FieldMask
	ExportFormat   string
}

// 预置模板
var (
	TemplateGDPR = ComplianceTemplate{
		Name:          "GDPR",
		Standard:      "GDPR",
		RetentionDays: 90,
		RedactionRules: []redaction.FieldMask{
			{FieldPath: "email", Mode: redaction.RedactionModeRedact},
			{FieldPath: "name", Mode: redaction.RedactionModeRedact},
			{FieldPath: "phone", Mode: redaction.RedactionModeRedact},
		},
	}

	TemplateSOX = ComplianceTemplate{
		Name:          "SOX",
		Standard:      "SOX",
		RetentionDays: 2555, // 7 years
		RedactionRules: []redaction.FieldMask{
			{FieldPath: "credit_card", Mode: redaction.RedactionModeRemove},
		},
	}

	TemplateHIPAA = ComplianceTemplate{
		Name:          "HIPAA",
		Standard:      "HIPAA",
		RetentionDays: 1825, // 5 years
		RedactionRules: []redaction.FieldMask{
			{FieldPath: "patient_id", Mode: redaction.RedactionModeHash},
			{FieldPath: "medical_record", Mode: redaction.RedactionModeEncrypt},
		},
	}
)

// GetTemplate 获取模板
func GetTemplate(name string) *ComplianceTemplate {
	templates := map[string]*ComplianceTemplate{
		"GDPR":  &TemplateGDPR,
		"SOX":   &TemplateSOX,
		"HIPAA": &TemplateHIPAA,
	}
	return templates[name]
}

// ListTemplates 列出所有模板
func ListTemplates() []ComplianceTemplate {
	return []ComplianceTemplate{TemplateGDPR, TemplateSOX, TemplateHIPAA}
}
