package desktop

import (
	"runtime"

	"github.com/egoist/mygo"
)

// Run starts the desktop application and returns when it quits.
//
// It is called by the shared binary when it is started without arguments;
// mygo generate also reaches it, with MYGO_GENERATE set, to write the
// TypeScript client and return before any window opens.
func Run() error {
	mygo.App.SetName("NCMconverter")
	mygo.Bind(&Converter{})
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(newMenu())
		openWindow()
	})
	// With a listener the app does not quit with its last window. macOS keeps
	// it in the Dock, where a click opens a window again; elsewhere there is no
	// Dock to come back from, so quit.
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			openWindow()
		}
	})
	return mygo.App.Run()
}

// openWindow creates the main window. Its background matches the page's --bg in
// both appearances, so switching does not flash white.
func openWindow() {
	mygo.NewWindow(mygo.WindowOptions{
		Title:           "NCMconverter",
		Width:           860,
		Height:          760,
		MinWidth:        620,
		MinHeight:       540,
		BackgroundColor: "light-dark(#f6f7f9, #16181d)",
		StateKey:        "main",
		URL:             "/",
	})
}

// newMenu builds the application menu.
func newMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "添加文件…", Accelerator: "CmdOrCtrl+O", Click: chooseFiles},
			{Label: "添加文件夹…", Accelerator: "CmdOrCtrl+Shift+O", Click: chooseDirectory},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	})
}

// chooseFiles asks for containers from the menu and hands them to the page.
//
// Dialogs block, and menu clicks run on the main thread, which draws the UI, so
// the dialog and the broadcast happen on a goroutine of their own.
func chooseFiles(*mygo.MenuItem, *mygo.Window) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:    "选择 .ncm 文件",
			Filters:  []mygo.FileFilter{{Name: "NCM 文件", Extensions: []string{"ncm"}}},
			Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		_ = Opened.Broadcast(paths)
	}()
}

// chooseDirectory asks for a directory from the menu and hands it to the page.
func chooseDirectory(*mygo.MenuItem, *mygo.Window) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:     "选择文件夹",
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		_ = Opened.Broadcast(paths)
	}()
}
