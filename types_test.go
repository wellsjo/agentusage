package agentusage

import "testing"

func TestSnapshotProvider(t *testing.T) {
	want := Provider{ID: ProviderIDCodex, Name: "Codex"}
	snapshot := Snapshot{Providers: []Provider{want}}

	got, ok := snapshot.Provider(ProviderIDCodex)
	if !ok || got.ID != want.ID || got.Name != want.Name {
		t.Fatalf("Provider() = (%+v, %v), want (%+v, true)", got, ok, want)
	}
	if _, ok := snapshot.Provider("missing"); ok {
		t.Fatal("Provider() found a missing provider")
	}
}

func TestProviderWindow(t *testing.T) {
	want := Window{ID: CodexWindowIDPrimary, Label: "5h window", UsedPercent: 12.5}
	provider := Provider{ID: ProviderIDCodex, Windows: []Window{want}}

	got, ok := provider.Window(CodexWindowIDPrimary)
	if !ok || got.ID != want.ID || got.Label != want.Label || got.UsedPercent != want.UsedPercent {
		t.Fatalf("Window() = (%+v, %v), want (%+v, true)", got, ok, want)
	}
	if _, ok := provider.Window("missing"); ok {
		t.Fatal("Window() found a missing window")
	}
}
