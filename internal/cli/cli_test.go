package cli

import (
	"testing"

	"sophonie/sono/internal/nodedist"
)

func sampleIndex() nodedist.Index {
	return nodedist.Index{
		{Version: "v22.5.1", LTS: nodedist.LTSFlag{}},
		{Version: "v20.15.0", LTS: nodedist.LTSFlag{IsLTS: true, Codename: "Iron"}},
		{Version: "v20.11.0", LTS: nodedist.LTSFlag{IsLTS: true, Codename: "Iron"}},
		{Version: "v18.20.0", LTS: nodedist.LTSFlag{IsLTS: true, Codename: "Hydrogen"}},
	}
}

func TestResolveRemoteNode(t *testing.T) {
	index := sampleIndex()
	cases := map[string]string{
		"20":       "v20.15.0",
		"20.11":    "v20.11.0",
		"v20.11.0": "v20.11.0",
		"lts":      "v20.15.0",
		"latest":   "v22.5.1",
		"LTS":      "v20.15.0",
	}
	for input, want := range cases {
		got, err := resolveRemoteNode(index, input)
		if err != nil {
			t.Fatalf("resolveRemoteNode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("resolveRemoteNode(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := resolveRemoteNode(index, "99"); err == nil {
		t.Fatal("expected error for unknown version")
	}
}

func TestResolveInstalledNode(t *testing.T) {
	installed := []string{"v20.11.0", "v20.15.0", "v22.5.1"}
	got, err := resolveInstalledNode(installed, "20")
	if err != nil {
		t.Fatalf("resolveInstalledNode: %v", err)
	}
	if got != "v20.15.0" {
		t.Fatalf("got %q, want v20.15.0", got)
	}
	if _, err := resolveInstalledNode(installed, "19"); err == nil {
		t.Fatal("expected error for missing installed version")
	}
}

func TestResolvePmVersion(t *testing.T) {
	versions := []string{"9.15.0", "9.15.9", "10.2.0", "9.14.0"}
	got, ok := resolvePmVersion("9.15", versions)
	if !ok || got != "9.15.9" {
		t.Fatalf("got %q ok=%v, want 9.15.9", got, ok)
	}
	got, ok = resolvePmVersion("", versions)
	if !ok || got != "10.2.0" {
		t.Fatalf("got %q ok=%v, want 10.2.0", got, ok)
	}
	if _, ok := resolvePmVersion("8", versions); ok {
		t.Fatal("expected no match for prefix 8")
	}
}

func TestSplitArgs(t *testing.T) {
	flags, positional := splitArgs([]string{"pnpm", "--use", "9.15", "--all"})
	if !flags["use"] || !flags["all"] {
		t.Fatalf("flags = %v", flags)
	}
	if len(positional) != 2 || positional[0] != "pnpm" || positional[1] != "9.15" {
		t.Fatalf("positional = %v", positional)
	}
}
