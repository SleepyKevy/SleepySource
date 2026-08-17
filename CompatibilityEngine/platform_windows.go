//go:build windows

package main

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func prepareBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procSetForegroundW   = user32.NewProc("SetForegroundWindow")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procMessageBoxW      = user32.NewProc("MessageBoxW")
	procLoadImageW       = user32.NewProc("LoadImageW")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
)

const (
	CS_HREDRAW      = 0x0002
	CS_VREDRAW      = 0x0001
	CW_USEDEFAULT   = 0x80000000
	WS_VISIBLE      = 0x10000000
	WM_DESTROY      = 0x0002
	WM_CLOSE        = 0x0010
	MB_OK           = 0x00000000
	MB_ICONERROR    = 0x00000010
	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x0010
	LR_DEFAULTSIZE  = 0x0040
)

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

var nativeApp *App

func utf16(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }

func ensureAppIconFile(app *App) string {
	iconPath := filepath.Join(app.dataDir, "app.ico")
	data, err := embedded.ReadFile("assets/app.ico")
	if err != nil {
		return ""
	}
	if existing, readErr := os.ReadFile(iconPath); readErr == nil && bytes.Equal(existing, data) {
		return iconPath
	}
	if err := os.MkdirAll(filepath.Dir(iconPath), 0755); err != nil {
		return ""
	}
	tmp, err := os.CreateTemp(filepath.Dir(iconPath), ".icon-*.tmp")
	if err != nil {
		return ""
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return ""
	}
	_ = tmp.Sync()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return ""
	}
	if err := commitTempFile(tmpName, iconPath, 4); err != nil {
		_ = os.Remove(tmpName)
		return ""
	}
	return iconPath
}

func loadIconFromFile(path string, cx, cy int32) syscall.Handle {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(utf16(path))), uintptr(IMAGE_ICON), uintptr(cx), uintptr(cy), uintptr(LR_LOADFROMFILE|LR_DEFAULTSIZE))
	return syscall.Handle(h)
}

func waitForDashboardReady(timeout time.Duration) bool {
	client := &http.Client{Timeout: 400 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(dashboardURL + "api/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func restoreExistingSleepySourceWindow() bool {
	className := utf16("SleepySourceEmbeddedWebViewWindow")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		return false
	}
	procShowWindow.Call(hwnd, swShow)
	procSetForegroundW.Call(hwnd)
	return true
}

func runNativeWindow(app *App) {
	nativeApp = app
	defer func() { nativeApp = nil }()
	if !waitForDashboardReady(8 * time.Second) {
		showFatal(appTitle, "SleepySource could not start its local Designer service on 127.0.0.1:17891.\n\nTry closing any older SleepySource instance and open it again.")
		return
	}
	if err := runEmbeddedDesigner(app); err != nil {
		showFatal(appTitle, err.Error())
	}
}

func openBrowser(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16("open"))), uintptr(unsafe.Pointer(utf16(rawURL))), 0, 0, 1)
}

func openFolder(path string) {
	_ = exec.Command("explorer.exe", path).Start()
}

func showFatal(title, message string) {
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(message))),
		uintptr(unsafe.Pointer(utf16(title))),
		MB_OK|MB_ICONERROR,
	)
}
