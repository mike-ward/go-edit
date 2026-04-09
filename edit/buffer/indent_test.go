package buffer

import "testing"

func TestDetectIndent(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want IndentStyle
	}{
		{
			"tabs",
			"func main() {\n\tfmt.Println()\n\tif true {\n\t\tx++\n\t}\n}\n",
			IndentStyle{UseTabs: true, Width: 4},
		},
		{
			"2 spaces",
			"def foo:\n  x = 1\n  if x:\n    y = 2\n    z = 3\n",
			IndentStyle{UseTabs: false, Width: 2},
		},
		{
			"4 spaces",
			"func main() {\n    fmt.Println()\n    if true {\n        x++\n    }\n}\n",
			IndentStyle{UseTabs: false, Width: 4},
		},
		{
			"no indent",
			"line1\nline2\nline3\n",
			IndentStyle{UseTabs: true, Width: 4},
		},
		{
			"empty buffer",
			"",
			IndentStyle{UseTabs: true, Width: 4},
		},
		{
			"mixed tabs dominate",
			"\tx\n\ty\n  z\n",
			IndentStyle{UseTabs: true, Width: 4},
		},
		{
			"single space ignored",
			" x\n y\n z\n",
			IndentStyle{UseTabs: true, Width: 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := FromBytes([]byte(tt.src))
			got := detectIndent(b)
			if got != tt.want {
				t.Errorf("detectIndent = %+v, want %+v", got, tt.want)
			}
		})
	}
}
