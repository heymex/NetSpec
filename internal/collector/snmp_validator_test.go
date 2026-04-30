package collector

import "testing"

func TestNormalizeInterfaceNameAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "twentyfive long form",
			in:   "TwentyFiveGigabitEthernet1/0/21",
			want: "tw1/0/21",
		},
		{
			name: "twentyfive gige form",
			in:   "TwentyFiveGigE1/0/21",
			want: "tw1/0/21",
		},
		{
			name: "twentyfive short form",
			in:   "Twe1/0/21",
			want: "tw1/0/21",
		},
		{
			name: "hundredgig long form",
			in:   "HundredGigabitEthernet1/0/49",
			want: "hu1/0/49",
		},
		{
			name: "hundredgig gige form",
			in:   "HundredGigE1/0/49",
			want: "hu1/0/49",
		},
		{
			name: "hundredgig short form",
			in:   "Hu1/0/49",
			want: "hu1/0/49",
		},
		{
			name: "ten gig remains stable",
			in:   "TenGigabitEthernet1/0/1",
			want: "te1/0/1",
		},
		{
			name: "port channel remains stable",
			in:   "Port-channel10",
			want: "po10",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeInterfaceName(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeInterfaceName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
