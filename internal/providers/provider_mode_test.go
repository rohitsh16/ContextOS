package providers

import (
	"strings"
	"testing"
)

func TestExecutionMetadataProvenance(t *testing.T) {
	metaMock := NewExecutionMetadata(RunTypeMock, ProviderModeMock, "2025-01-31")
	if metaMock.IsBilled() {
		t.Errorf("mock run should never report as billed")
	}
	if !strings.HasPrefix(metaMock.ExecutionID, "exec_") {
		t.Errorf("expected execution ID prefix exec_, got %s", metaMock.ExecutionID)
	}

	metaReal := NewExecutionMetadata(RunTypeReal, ProviderModeReal, "2025-01-31")
	if !metaReal.IsBilled() {
		t.Errorf("real provider run with real mode must report as billed")
	}

	metaSynthetic := NewExecutionMetadata(RunTypeSynthetic, ProviderModeMock, "2025-01-31")
	if metaSynthetic.IsBilled() {
		t.Errorf("synthetic run must not report as billed")
	}
}
