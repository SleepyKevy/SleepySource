//go:build windows

package main

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	wmApp                     = 0x8000
	trayCallbackMessage       = wmApp + 17
	designerFatalCloseMessage = wmApp + 18
	trayIconID                = 1
	wmNull                    = 0x0000
	wmContextMenu             = 0x007B
	wmLButtonUp               = 0x0202
	wmLButtonDblClk           = 0x0203
	wmRButtonUp               = 0x0205
	ninSelect                 = 0x0400
	ninKeySelect              = 0x0401
	swHide                    = 0
	swShow                    = 5
	swRestore                 = 9
	nimAdd                    = 0x00000000
	nimModify                 = 0x00000001
	nimDelete                 = 0x00000002
	nimSetVersion             = 0x00000004
	nifMessage                = 0x00000001
	nifIcon                   = 0x00000002
	nifTip                    = 0x00000004
	nifShowTip                = 0x00000080
	notifyIconVersion4        = 4
	mfString                  = 0x00000000
	mfSeparator               = 0x00000800
	tpmRightButton            = 0x0002
	tpmReturnCmd              = 0x0100
	trayMenuOpen              = 2101
	trayMenuOpenData          = 2102
	trayMenuExit              = 2103
)

type trayGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         trayGUID
	HBalloonIcon     syscall.Handle
}

type trayPoint struct {
	X int32
	Y int32
}

var (
	shell32Tray                 = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIconW        = shell32Tray.NewProc("Shell_NotifyIconW")
	procCreatePopupMenuTray     = user32.NewProc("CreatePopupMenu")
	procAppendMenuWTray         = user32.NewProc("AppendMenuW")
	procTrackPopupMenuTray      = user32.NewProc("TrackPopupMenu")
	procDestroyMenuTray         = user32.NewProc("DestroyMenu")
	procGetCursorPosTray        = user32.NewProc("GetCursorPos")
	procSetForegroundWindowTray = user32.NewProc("SetForegroundWindow")
	procIsIconicTray            = user32.NewProc("IsIconic")
	procPostMessageWTray        = user32.NewProc("PostMessageW")
	procDestroyIconTray         = user32.NewProc("DestroyIcon")
	trayWindow                  atomic.Uintptr
	trayInstalled               atomic.Bool
	trayIcon                    atomic.Uintptr
)

func fillUTF16(dst []uint16, s string) {
	v := syscall.StringToUTF16(s)
	if len(v) > len(dst) {
		v = v[:len(dst)]
		if len(v) > 0 {
			v[len(v)-1] = 0
		}
	}
	copy(dst, v)
}

func makeTrayData(hwnd syscall.Handle, icon syscall.Handle) notifyIconDataW {
	data := notifyIconDataW{
		HWnd:             hwnd,
		UID:              trayIconID,
		UFlags:           nifMessage | nifIcon | nifTip | nifShowTip,
		UCallbackMessage: trayCallbackMessage,
		HIcon:            icon,
	}
	data.CbSize = uint32(unsafe.Sizeof(data))
	fillUTF16(data.SzTip[:], "SleepySource "+appVersion+" — Made by SleepyKev • 2026")
	return data
}

func installSystemTray(app *App, hwnd syscall.Handle) error {
	if hwnd == 0 {
		return fmt.Errorf("invalid SleepySource window handle")
	}
	iconPath := ensureAppIconFile(app)
	icon := loadIconFromFile(iconPath, 32, 32)
	if icon == 0 {
		return fmt.Errorf("could not load the SleepySource tray icon")
	}
	data := makeTrayData(hwnd, icon)
	r, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if r == 0 {
		procDestroyIconTray.Call(uintptr(icon))
		return fmt.Errorf("Windows could not add the SleepySource tray icon: %v", err)
	}
	data.UVersion = notifyIconVersion4
	procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&data)))
	trayWindow.Store(uintptr(hwnd))
	trayIcon.Store(uintptr(icon))
	trayInstalled.Store(true)
	return nil
}

func removeSystemTray() {
	if !trayInstalled.Swap(false) {
		return
	}
	hwnd := syscall.Handle(trayWindow.Load())
	icon := syscall.Handle(trayIcon.Load())
	data := makeTrayData(hwnd, icon)
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	if icon != 0 {
		procDestroyIconTray.Call(uintptr(icon))
	}
	trayWindow.Store(0)
	trayIcon.Store(0)
}

func restoreSleepySourceFromTray() {
	hwnd := syscall.Handle(trayWindow.Load())
	if hwnd == 0 {
		return
	}
	if iconic, _, _ := procIsIconicTray.Call(uintptr(hwnd)); iconic != 0 {
		procShowWindow.Call(uintptr(hwnd), swRestore)
	} else {
		procShowWindow.Call(uintptr(hwnd), swShow)
	}
	procSetForegroundWindowTray.Call(uintptr(hwnd))
}

func popupSystemTrayMenu(hwnd syscall.Handle) {
	menu, _, _ := procCreatePopupMenuTray.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenuTray.Call(menu)
	procAppendMenuWTray.Call(menu, mfString, trayMenuOpen, uintptr(unsafe.Pointer(utf16("Open SleepySource"))))
	procAppendMenuWTray.Call(menu, mfString, trayMenuOpenData, uintptr(unsafe.Pointer(utf16("Open SleepySource_Data"))))
	procAppendMenuWTray.Call(menu, mfSeparator, 0, 0)
	procAppendMenuWTray.Call(menu, mfString, trayMenuExit, uintptr(unsafe.Pointer(utf16("Exit SleepySource"))))
	var p trayPoint
	if ok, _, _ := procGetCursorPosTray.Call(uintptr(unsafe.Pointer(&p))); ok == 0 {
		return
	}
	procSetForegroundWindowTray.Call(uintptr(hwnd))
	cmd, _, _ := procTrackPopupMenuTray.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(p.X), uintptr(p.Y), 0, uintptr(hwnd), 0)
	procPostMessageWTray.Call(uintptr(hwnd), wmNull, 0, 0)
	switch cmd {
	case trayMenuOpen:
		restoreSleepySourceFromTray()
	case trayMenuOpenData:
		if nativeApp != nil {
			go openFolder(nativeApp.dataDir)
		}
	case trayMenuExit:
		removeSystemTray()
		procDestroyWindow.Call(uintptr(hwnd))
	}
}

func handleSystemTrayMessage(hwnd syscall.Handle, lParam uintptr) bool {
	// NOTIFYICON_VERSION_4 reports the event in LOWORD(lParam). Older shells
	// may pass the event directly, so accept both shapes.
	event := uint32(lParam & 0xffff)
	if event == 0 {
		event = uint32(lParam)
	}
	switch event {
	case wmLButtonUp, wmLButtonDblClk, ninSelect, ninKeySelect:
		restoreSleepySourceFromTray()
		return true
	case wmRButtonUp, wmContextMenu:
		popupSystemTrayMenu(hwnd)
		return true
	default:
		return false
	}
}
