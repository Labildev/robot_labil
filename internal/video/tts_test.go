package video

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPiperTTSAvailabilityAndArgs(t *testing.T) {
	tmp := t.TempDir()
	fakeBin := filepath.Join(tmp, "piper.exe")
	fakeModel := filepath.Join(tmp, "voice.onnx")

	pMissing := NewPiperTTS(true, fakeBin, fakeModel)
	if pMissing.Available() {
		t.Fatalf("expected Available()=false when binary and model do not exist")
	}

	if err := os.WriteFile(fakeBin, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	if err := os.WriteFile(fakeModel, []byte("onnx"), 0o644); err != nil {
		t.Fatalf("write fake model: %v", err)
	}

	pReady := NewPiperTTS(true, fakeBin, fakeModel)
	if !pReady.Available() {
		t.Fatalf("expected Available()=true when binary and model exist")
	}

	gotArgs := pReady.BuildArgs("out/voice.wav")
	wantArgs := []string{"--model", fakeModel, "--output_file", "out/voice.wav"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("unexpected Piper args: got %v want %v", gotArgs, wantArgs)
	}

	pDisabled := NewPiperTTS(false, fakeBin, fakeModel)
	if pDisabled.Available() {
		t.Fatalf("expected Available()=false when disabled")
	}
}
