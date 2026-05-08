module github.com/AudDMusic/audd-go/examples/scan_and_rename

go 1.24

require (
	github.com/AudDMusic/audd-go v0.0.0
	github.com/bogem/id3v2/v2 v2.1.4
	golang.org/x/sync v0.10.0
)

require golang.org/x/text v0.3.8 // indirect

replace github.com/AudDMusic/audd-go => ../..
