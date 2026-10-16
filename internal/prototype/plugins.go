package prototype

import (
	"log"
	"time"

	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	ulua "github.com/nevdelap/mdiff/internal/micro/lua"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/micro/shell"
	"github.com/nevdelap/mdiff/internal/micro/util"
	"github.com/yuin/gopher-lua"
	"layeh.com/gopher-luar"
)

// initMicroLua installs the same import bridge used by Micro's entrypoint.
// Pane and tab lookups are backed by mdiff's two-pane app rather than the
// stock global tab layout.
func initMicroLua(app *App) {
	ulua.L.SetGlobal("import", luar.New(ulua.L, func(pkg string) *lua.LTable {
		switch pkg {
		case "micro":
			return luaImportMicro(app)
		case "micro/shell":
			return luaImportMicroShell()
		case "micro/buffer":
			return luaImportMicroBuffer()
		case "micro/config":
			return luaImportMicroConfig(app)
		case "micro/util":
			return luaImportMicroUtil()
		default:
			return ulua.Import(pkg)
		}
	}))
}

func luaImportMicro(app *App) *lua.LTable {
	pkg := ulua.L.NewTable()
	ulua.L.SetField(pkg, "TermMessage", luar.New(ulua.L, screen.TermMessage))
	ulua.L.SetField(pkg, "TermError", luar.New(ulua.L, screen.TermError))
	ulua.L.SetField(pkg, "InfoBar", luar.New(ulua.L, action.GetInfoBar))
	ulua.L.SetField(pkg, "Log", luar.New(ulua.L, log.Println))
	ulua.L.SetField(pkg, "SetStatusInfoFn", luar.New(ulua.L, display.SetStatusInfoFnLua))
	ulua.L.SetField(pkg, "CurPane", luar.New(ulua.L, func() *action.BufPane {
		if app.helpVisible && app.helpPane != nil {
			return app.helpPane
		}
		if app.focusedPane() == nil {
			return nil
		}
		return app.focusedPane().h
	}))
	ulua.L.SetField(pkg, "CurTab", luar.New(ulua.L, func() *action.Tab { return app.microTab }))
	ulua.L.SetField(pkg, "Tabs", luar.New(ulua.L, func() *action.TabList { return action.Tabs }))
	ulua.L.SetField(pkg, "After", luar.New(ulua.L, func(delay time.Duration, fn func()) {
		time.AfterFunc(delay, func() {
			select {
			case app.pluginTimers <- fn:
			case <-app.pluginStop:
			}
		})
	}))
	return pkg
}

func luaImportMicroConfig(app *App) *lua.LTable {
	pkg := ulua.L.NewTable()
	for name, fn := range map[string]any{
		"MakeCommand": action.MakeCommand, "FileComplete": buffer.FileComplete,
		"HelpComplete": action.HelpComplete, "OptionComplete": action.OptionComplete,
		"OptionValueComplete": action.OptionValueComplete, "TryBindKey": action.TryBindKeyPlug,
		"Reload": app.rejectPluginReload, "AddRuntimeFileFromMemory": config.PluginAddRuntimeFileFromMemory,
		"AddRuntimeFilesFromDirectory": config.PluginAddRuntimeFilesFromDirectory,
		"AddRuntimeFile":               config.PluginAddRuntimeFile, "ListRuntimeFiles": config.PluginListRuntimeFiles,
		"ReadRuntimeFile": config.PluginReadRuntimeFile, "NewRTFiletype": config.NewRTFiletype,
		"RegisterCommonOption": config.RegisterCommonOptionPlug,
		"RegisterGlobalOption": config.RegisterGlobalOptionPlug,
		"GetGlobalOption":      config.GetGlobalOption, "SetGlobalOption": action.SetGlobalOptionPlug,
		"SetGlobalOptionNative": action.SetGlobalOptionNativePlug,
	} {
		ulua.L.SetField(pkg, name, luar.New(ulua.L, fn))
	}
	for name, value := range map[string]any{
		"NoComplete": nil, "RTColorscheme": config.RTColorscheme, "RTSyntax": config.RTSyntax,
		"RTHelp": config.RTHelp, "RTPlugin": config.RTPlugin, "ConfigDir": config.ConfigDir,
	} {
		ulua.L.SetField(pkg, name, luar.New(ulua.L, value))
	}
	return pkg
}

func luaImportMicroShell() *lua.LTable {
	pkg := ulua.L.NewTable()
	for name, fn := range map[string]any{
		"ExecCommand": shell.ExecCommand, "RunCommand": shell.RunCommand,
		"RunBackgroundShell": shell.RunBackgroundShell, "RunInteractiveShell": shell.RunInteractiveShell,
		"JobStart": shell.JobStart, "JobSpawn": shell.JobSpawn, "JobStop": shell.JobStop,
		"JobSend": shell.JobSend, "RunTermEmulator": action.RunTermEmulator,
		"TermEmuSupported": action.TermEmuSupported,
	} {
		ulua.L.SetField(pkg, name, luar.New(ulua.L, fn))
	}
	return pkg
}

func luaImportMicroBuffer() *lua.LTable {
	pkg := ulua.L.NewTable()
	for name, fn := range map[string]any{
		"NewMessage": buffer.NewMessage, "NewMessageAtLine": buffer.NewMessageAtLine,
		"MTInfo": buffer.MTInfo, "MTWarning": buffer.MTWarning, "MTError": buffer.MTError,
		"ByteOffset": buffer.ByteOffset, "Log": buffer.WriteLog, "LogBuf": buffer.GetLogBuf,
		"NewBuffer": func(text, path string) *buffer.Buffer {
			return buffer.NewBufferFromString(text, path, buffer.BTDefault)
		},
		"NewBufferFromFile": func(path string) (*buffer.Buffer, error) {
			return buffer.NewBufferFromFile(path, buffer.BTDefault)
		},
		"Loc":  func(x, y int) buffer.Loc { return buffer.Loc{X: x, Y: y} },
		"SLoc": func(line, row int) display.SLoc { return display.SLoc{Line: line, Row: row} },
	} {
		ulua.L.SetField(pkg, name, luar.New(ulua.L, fn))
	}
	for name, value := range map[string]any{
		"BTDefault": buffer.BTDefault.Kind, "BTHelp": buffer.BTHelp.Kind,
		"BTLog": buffer.BTLog.Kind, "BTScratch": buffer.BTScratch.Kind,
		"BTRaw": buffer.BTRaw.Kind, "BTInfo": buffer.BTInfo.Kind,
	} {
		ulua.L.SetField(pkg, name, luar.New(ulua.L, value))
	}
	return pkg
}

func luaImportMicroUtil() *lua.LTable {
	pkg := ulua.L.NewTable()
	for name, fn := range map[string]any{
		"RuneAt": util.LuaRuneAt, "GetLeadingWhitespace": util.LuaGetLeadingWhitespace,
		"IsWordChar": util.LuaIsWordChar, "String": util.String, "Unzip": util.Unzip,
		"Version": util.Version, "SemVersion": util.SemVersion, "HttpRequest": util.HttpRequest,
		"CharacterCountInString": util.CharacterCountInString,
		"RuneStr":                func(r rune) string { return string(r) },
	} {
		ulua.L.SetField(pkg, name, luar.New(ulua.L, fn))
	}
	return pkg
}
