package mdns

import "testing"

func TestIsVirtual(t *testing.T) {
	tests := []struct {
		name    string
		virtual bool
	}{
		{"docker0", true},
		{"br-abc123def456", true},
		{"vethfae16f2", true},
		{"virbr0", true},
		{"tun0", true},
		{"tap0", true},
		{"eth0", false},
		{"enp0s31f6", false},
		{"wlp0s20f3", false},
		{"enx387c7618f753", false},
		{"wlan0", false},
		{"bond0", false},
		{"wwan0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isVirtual(tt.name); got != tt.virtual {
				t.Errorf("isVirtual(%q) = %v, want %v", tt.name, got, tt.virtual)
			}
		})
	}
}

func TestPhysicalInterfaces(t *testing.T) {
	ifaces, err := PhysicalInterfaces()
	if err != nil {
		t.Fatalf("PhysicalInterfaces() error: %v", err)
	}

	// Should return at least one interface on any real system.
	if len(ifaces) == 0 {
		t.Error("PhysicalInterfaces() returned no interfaces")
	}

	// None should match virtual patterns.
	for _, iface := range ifaces {
		if isVirtual(iface) {
			t.Errorf("PhysicalInterfaces() returned virtual interface: %s", iface)
		}
	}
}
