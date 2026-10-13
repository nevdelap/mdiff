module github.com/nevdelap/mdiff

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/creack/pty v1.1.24
	github.com/dustin/go-humanize v1.1.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/mattn/go-runewidth v0.0.31
	github.com/micro-editor/json5 v1.0.1-micro
	github.com/micro-editor/tcell/v2 v2.0.13
	github.com/micro-editor/terminal v0.0.0-20250324214352-e587e959c6b5
	github.com/mitchellh/go-homedir v1.1.0
	github.com/sergi/go-diff v1.4.0
	github.com/stretchr/testify v1.12.1
	github.com/yuin/gopher-lua v1.1.2
	github.com/zyedidia/clipper v0.1.1
	github.com/zyedidia/glob v0.0.0-20170209203856-dd4023a66dc3
	golang.org/x/text v0.43.0
	gopkg.in/yaml.v2 v2.4.0
	layeh.com/gopher-luar v1.0.11
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/npat-efault/poller v2.0.0+incompatible // indirect
	github.com/zyedidia/poller v2.0.0+incompatible // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.47.0 // indirect
)

replace github.com/kballard/go-shellquote => github.com/micro-editor/go-shellquote v0.0.0-20250101105543-feb6c39314f5

replace layeh.com/gopher-luar v1.0.11 => github.com/layeh/gopher-luar v1.0.11

go 1.26.0
