//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	wmSize                         = 0x0005
	wsOverlappedWindow      uint32 = 0x00CF0000
	coinitApartmentThreaded        = 0x2
	swShowNormal                   = 1
	webViewRuntimeInstalled        = 0

	wvSOK          = uintptr(0)
	wvSFalse       = uintptr(1)
	wvENoInterface = uintptr(0x80004002)
	wvEPointer     = uintptr(0x80004003)
)

var (
	ole32WV              = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeExWV = ole32WV.NewProc("CoInitializeEx")
	procCoUninitializeWV = ole32WV.NewProc("CoUninitialize")
	procCoTaskMemAllocWV = ole32WV.NewProc("CoTaskMemAlloc")
	procGetClientRectWV  = user32.NewProc("GetClientRect")
	procPostMessageWVW   = user32.NewProc("PostMessageW")
)

type wvGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	iidIUnknown    = wvGUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidEnvironment = wvGUID{0xB96D755E, 0x0319, 0x4E92, [8]byte{0xA2, 0x96, 0x23, 0x43, 0x6F, 0x46, 0xA1, 0xFC}}
	iidEnvHandler  = wvGUID{0x4E8A3389, 0xC9D8, 0x4BD2, [8]byte{0xB6, 0xB5, 0x12, 0x4F, 0xEE, 0x6C, 0xC1, 0x4D}}
	iidCtlHandler  = wvGUID{0x6C4819F3, 0xC9B7, 0x4260, [8]byte{0x81, 0x27, 0xC9, 0xF5, 0xBD, 0xE7, 0xF6, 0x8C}}
	iidEnvOptions  = wvGUID{0x2FDE08A8, 0x1E9A, 0x4766, [8]byte{0x8C, 0x05, 0x95, 0xA9, 0xCE, 0xB9, 0xD1, 0xC5}}
)

func guidEqualPtr(p uintptr, want *wvGUID) bool {
	if p == 0 || want == nil {
		return false
	}
	return *(*wvGUID)(unsafe.Pointer(p)) == *want
}

type wvRect struct{ Left, Top, Right, Bottom int32 }

type wvIUnknownVtbl struct{ QueryInterface, AddRef, Release uintptr }

type wvEnvironmentVtbl struct {
	wvIUnknownVtbl
	CreateCoreWebView2Controller, CreateWebResourceResponse, GetBrowserVersionString uintptr
	AddNewBrowserVersionAvailable, RemoveNewBrowserVersionAvailable                  uintptr
}
type wvEnvironment struct{ vtbl *wvEnvironmentVtbl }

type wvControllerVtbl struct {
	wvIUnknownVtbl
	GetIsVisible, PutIsVisible, GetBounds, PutBounds, GetZoomFactor, PutZoomFactor              uintptr
	AddZoomFactorChanged, RemoveZoomFactorChanged, SetBoundsAndZoomFactor, MoveFocus            uintptr
	AddMoveFocusRequested, RemoveMoveFocusRequested, AddGotFocus, RemoveGotFocus                uintptr
	AddLostFocus, RemoveLostFocus, AddAcceleratorKeyPressed, RemoveAcceleratorKeyPressed        uintptr
	GetParentWindow, PutParentWindow, NotifyParentWindowPositionChanged, Close, GetCoreWebView2 uintptr
}
type wvController struct{ vtbl *wvControllerVtbl }

type wvCoreVtbl struct {
	wvIUnknownVtbl
	GetSettings, GetSource, Navigate uintptr
}
type wvCore struct{ vtbl *wvCoreVtbl }

type wvEnvHandlerVtbl struct {
	wvIUnknownVtbl
	Invoke uintptr
}
type wvEnvHandler struct {
	vtbl *wvEnvHandlerVtbl
	refs uint32
	host *embeddedDesignerHost
}

type wvControllerHandlerVtbl struct {
	wvIUnknownVtbl
	Invoke uintptr
}
type wvControllerHandler struct {
	vtbl *wvControllerHandlerVtbl
	refs uint32
	host *embeddedDesignerHost
}

// ICoreWebView2EnvironmentOptions (the original options interface). The
// internal Evergreen runtime entrypoint expects this COM object even when all
// settings are defaults; passing nil can leave some runtime versions waiting
// without ever creating the controller.
type wvEnvOptionsVtbl struct {
	wvIUnknownVtbl
	GetAdditionalBrowserArguments, PutAdditionalBrowserArguments                         uintptr
	GetLanguage, PutLanguage                                                             uintptr
	GetTargetCompatibleBrowserVersion, PutTargetCompatibleBrowserVersion                 uintptr
	GetAllowSingleSignOnUsingOSPrimaryAccount, PutAllowSingleSignOnUsingOSPrimaryAccount uintptr
}
type wvEnvOptions struct {
	vtbl *wvEnvOptionsVtbl
	refs uint32
}

type embeddedDesignerHost struct {
	hwnd       syscall.Handle
	env        *wvEnvironment
	controller *wvController
	core       *wvCore
	envHandler *wvEnvHandler
	ctlHandler *wvControllerHandler
	envOptions *wvEnvOptions
	ready      atomic.Bool
	errMu      sync.Mutex
	initErr    error
}

var activeEmbeddedDesigner *embeddedDesignerHost

func hresultFailed(v uintptr) bool { return int32(uint32(v)) < 0 }
func wvCall(fn uintptr, args ...uintptr) uintptr {
	if fn == 0 {
		return ^uintptr(0)
	}
	r1, _, _ := syscall.SyscallN(fn, args...)
	return r1
}

func (h *embeddedDesignerHost) setError(err error) {
	if err == nil {
		return
	}
	h.errMu.Lock()
	if h.initErr == nil {
		h.initErr = err
	}
	h.errMu.Unlock()
}
func (h *embeddedDesignerHost) getError() error {
	h.errMu.Lock()
	defer h.errMu.Unlock()
	return h.initErr
}

func comQIBase(this, riid, object uintptr, ownIID *wvGUID, addRef func(uintptr) uintptr) uintptr {
	if object == 0 {
		return wvEPointer
	}
	*(*uintptr)(unsafe.Pointer(object)) = 0
	if guidEqualPtr(riid, &iidIUnknown) || guidEqualPtr(riid, ownIID) {
		*(*uintptr)(unsafe.Pointer(object)) = this
		addRef(this)
		return wvSOK
	}
	return wvENoInterface
}
func wvEnvQI(this, riid, object uintptr) uintptr {
	return comQIBase(this, riid, object, &iidEnvHandler, wvEnvAddRef)
}
func wvCtlQI(this, riid, object uintptr) uintptr {
	return comQIBase(this, riid, object, &iidCtlHandler, wvCtlAddRef)
}
func wvOptsQI(this, riid, object uintptr) uintptr {
	return comQIBase(this, riid, object, &iidEnvOptions, wvOptsAddRef)
}
func wvEnvAddRef(this uintptr) uintptr {
	return uintptr(atomic.AddUint32(&(*wvEnvHandler)(unsafe.Pointer(this)).refs, 1))
}
func wvEnvRelease(this uintptr) uintptr {
	return uintptr(atomic.AddUint32(&(*wvEnvHandler)(unsafe.Pointer(this)).refs, ^uint32(0)))
}
func wvCtlAddRef(this uintptr) uintptr {
	return uintptr(atomic.AddUint32(&(*wvControllerHandler)(unsafe.Pointer(this)).refs, 1))
}
func wvCtlRelease(this uintptr) uintptr {
	return uintptr(atomic.AddUint32(&(*wvControllerHandler)(unsafe.Pointer(this)).refs, ^uint32(0)))
}
func wvOptsAddRef(this uintptr) uintptr {
	return uintptr(atomic.AddUint32(&(*wvEnvOptions)(unsafe.Pointer(this)).refs, 1))
}
func wvOptsRelease(this uintptr) uintptr {
	return uintptr(atomic.AddUint32(&(*wvEnvOptions)(unsafe.Pointer(this)).refs, ^uint32(0)))
}

func allocCOMString(s string) *uint16 {
	buf := syscall.StringToUTF16(s)
	bytes := uintptr(len(buf) * 2)
	p, _, _ := procCoTaskMemAllocWV.Call(bytes)
	if p == 0 {
		return nil
	}
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(buf))
	copy(dst, buf)
	return (*uint16)(unsafe.Pointer(p))
}
func setOutCOMString(out uintptr, value string) uintptr {
	if out == 0 {
		return wvEPointer
	}
	p := allocCOMString(value)
	if p == nil {
		return uintptr(0x8007000E)
	}
	*(**uint16)(unsafe.Pointer(out)) = p
	return wvSOK
}
func wvOptsGetArgs(this, out uintptr) uintptr            { return setOutCOMString(out, "") }
func wvOptsPutArgs(this, value uintptr) uintptr          { return wvSFalse }
func wvOptsGetLanguage(this, out uintptr) uintptr        { return setOutCOMString(out, "en-US") }
func wvOptsPutLanguage(this, value uintptr) uintptr      { return wvSFalse }
func wvOptsGetTargetVersion(this, out uintptr) uintptr   { return setOutCOMString(out, "86.0.616.0") }
func wvOptsPutTargetVersion(this, value uintptr) uintptr { return wvSFalse }
func wvOptsGetSSO(this, out uintptr) uintptr {
	if out == 0 {
		return wvEPointer
	}
	*(*int32)(unsafe.Pointer(out)) = 0
	return wvSOK
}
func wvOptsPutSSO(this, value uintptr) uintptr { return wvSFalse }

func wvEnvInvoke(this, errorCode, createdEnvironment uintptr) uintptr {
	h := (*wvEnvHandler)(unsafe.Pointer(this))
	host := h.host
	if hresultFailed(errorCode) || createdEnvironment == 0 {
		host.setError(fmt.Errorf("WebView2 Runtime could not create the SleepySource Designer environment (HRESULT 0x%08X).", uint32(errorCode)))
		procPostMessageWVW.Call(uintptr(host.hwnd), designerFatalCloseMessage, 0, 0)
		return 0
	}
	// The internal runtime callback can hand back an implementation pointer that
	// must be QI'd to the public ICoreWebView2Environment interface before using
	// its vtable. This is the step v5.16.2 was missing.
	raw := (*wvEnvironment)(unsafe.Pointer(createdEnvironment))
	var canonical *wvEnvironment
	hr := wvCall(raw.vtbl.QueryInterface, createdEnvironment, uintptr(unsafe.Pointer(&iidEnvironment)), uintptr(unsafe.Pointer(&canonical)))
	if hresultFailed(hr) || canonical == nil {
		host.setError(fmt.Errorf("WebView2 could not expose its Designer environment (HRESULT 0x%08X).", uint32(hr)))
		procPostMessageWVW.Call(uintptr(host.hwnd), designerFatalCloseMessage, 0, 0)
		return 0
	}
	host.env = canonical
	host.ctlHandler = newWVControllerHandler(host)
	hr = wvCall(host.env.vtbl.CreateCoreWebView2Controller, uintptr(unsafe.Pointer(host.env)), uintptr(host.hwnd), uintptr(unsafe.Pointer(host.ctlHandler)))
	if hresultFailed(hr) {
		host.setError(fmt.Errorf("WebView2 Runtime could not create the SleepySource Designer control (HRESULT 0x%08X).", uint32(hr)))
		procPostMessageWVW.Call(uintptr(host.hwnd), designerFatalCloseMessage, 0, 0)
	}
	return 0
}

func wvControllerInvoke(this, errorCode, createdController uintptr) uintptr {
	h := (*wvControllerHandler)(unsafe.Pointer(this))
	host := h.host
	if hresultFailed(errorCode) || createdController == 0 {
		host.setError(fmt.Errorf("WebView2 Runtime could not attach the SleepySource Designer (HRESULT 0x%08X).", uint32(errorCode)))
		procPostMessageWVW.Call(uintptr(host.hwnd), designerFatalCloseMessage, 0, 0)
		return 0
	}
	host.controller = (*wvController)(unsafe.Pointer(createdController))
	wvCall(host.controller.vtbl.AddRef, createdController)
	var core *wvCore
	hr := wvCall(host.controller.vtbl.GetCoreWebView2, uintptr(unsafe.Pointer(host.controller)), uintptr(unsafe.Pointer(&core)))
	if hresultFailed(hr) || core == nil {
		host.setError(fmt.Errorf("WebView2 Runtime could not expose the Designer view (HRESULT 0x%08X).", uint32(hr)))
		procPostMessageWVW.Call(uintptr(host.hwnd), designerFatalCloseMessage, 0, 0)
		return 0
	}
	host.core = core
	wvCall(host.controller.vtbl.PutIsVisible, uintptr(unsafe.Pointer(host.controller)), 1)
	host.resize()
	url := syscall.StringToUTF16Ptr(dashboardURL)
	hr = wvCall(host.core.vtbl.Navigate, uintptr(unsafe.Pointer(host.core)), uintptr(unsafe.Pointer(url)))
	if hresultFailed(hr) {
		host.setError(fmt.Errorf("WebView2 could not navigate to the SleepySource Designer (HRESULT 0x%08X).", uint32(hr)))
		procPostMessageWVW.Call(uintptr(host.hwnd), designerFatalCloseMessage, 0, 0)
		return 0
	}
	host.ready.Store(true)
	return 0
}

var wvEnvVtblInstance = wvEnvHandlerVtbl{wvIUnknownVtbl{syscall.NewCallback(wvEnvQI), syscall.NewCallback(wvEnvAddRef), syscall.NewCallback(wvEnvRelease)}, syscall.NewCallback(wvEnvInvoke)}
var wvCtlVtblInstance = wvControllerHandlerVtbl{wvIUnknownVtbl{syscall.NewCallback(wvCtlQI), syscall.NewCallback(wvCtlAddRef), syscall.NewCallback(wvCtlRelease)}, syscall.NewCallback(wvControllerInvoke)}
var wvOptsVtblInstance = wvEnvOptionsVtbl{wvIUnknownVtbl{syscall.NewCallback(wvOptsQI), syscall.NewCallback(wvOptsAddRef), syscall.NewCallback(wvOptsRelease)}, syscall.NewCallback(wvOptsGetArgs), syscall.NewCallback(wvOptsPutArgs), syscall.NewCallback(wvOptsGetLanguage), syscall.NewCallback(wvOptsPutLanguage), syscall.NewCallback(wvOptsGetTargetVersion), syscall.NewCallback(wvOptsPutTargetVersion), syscall.NewCallback(wvOptsGetSSO), syscall.NewCallback(wvOptsPutSSO)}

func newWVEnvHandler(host *embeddedDesignerHost) *wvEnvHandler {
	return &wvEnvHandler{vtbl: &wvEnvVtblInstance, refs: 1, host: host}
}
func newWVControllerHandler(host *embeddedDesignerHost) *wvControllerHandler {
	return &wvControllerHandler{vtbl: &wvCtlVtblInstance, refs: 1, host: host}
}
func newWVEnvOptions() *wvEnvOptions { return &wvEnvOptions{vtbl: &wvOptsVtblInstance, refs: 1} }

func (h *embeddedDesignerHost) resize() {
	if h == nil || h.controller == nil || h.hwnd == 0 {
		return
	}
	var r wvRect
	ok, _, _ := procGetClientRectWV.Call(uintptr(h.hwnd), uintptr(unsafe.Pointer(&r)))
	if ok == 0 {
		return
	}
	wvCall(h.controller.vtbl.PutBounds, uintptr(unsafe.Pointer(h.controller)), uintptr(unsafe.Pointer(&r)))
	wvCall(h.controller.vtbl.NotifyParentWindowPositionChanged, uintptr(unsafe.Pointer(h.controller)))
}
func embeddedWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case trayCallbackMessage:
		if handleSystemTrayMessage(hwnd, lParam) {
			return 0
		}
	case designerFatalCloseMessage:
		// Internal startup/runtime failures close the native window immediately.
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmSize:
		if activeEmbeddedDesigner != nil {
			activeEmbeddedDesigner.resize()
		}
		return 0
	case WM_CLOSE:
		// Keep SleepySource alive in the system tray so the local webhook server and
		// Cloudflare Quick Tunnel keep the same public URL when the main window closes.
		// A true shutdown is still available from the tray menu: Exit SleepySource.
		if trayInstalled.Load() {
			procShowWindow.Call(uintptr(hwnd), swHide)
			return 0
		}
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case WM_DESTROY:
		removeSystemTray()
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

var embeddedWndProcCallback = syscall.NewCallback(embeddedWndProc)

func parseVersionParts(s string) []int {
	chunks := strings.Split(s, ".")
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i], _ = strconv.Atoi(c)
	}
	return out
}
func versionGreater(a, b string) bool {
	aa, bb := parseVersionParts(a), parseVersionParts(b)
	n := len(aa)
	if len(bb) > n {
		n = len(bb)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(aa) {
			x = aa[i]
		}
		if i < len(bb) {
			y = bb[i]
		}
		if x != y {
			return x > y
		}
	}
	return a > b
}
func findEmbeddedBrowserWebViewDLL() string {
	roots := []string{filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "EdgeWebView", "Application"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "EdgeWebView", "Application"), filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "EdgeWebView", "Application")}
	type candidate struct{ version, path string }
	var found []candidate
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(root, e.Name(), "EBWebView", "x64", "EmbeddedBrowserWebView.dll")
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				found = append(found, candidate{e.Name(), p})
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return versionGreater(found[i].version, found[j].version) })
	if len(found) > 0 {
		return found[0].path
	}
	return ""
}
func createEmbeddedWindow(app *App) (syscall.Handle, error) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := utf16("SleepySourceEmbeddedWebViewWindow")
	iconPath := ensureAppIconFile(app)
	bigIcon := loadIconFromFile(iconPath, 256, 256)
	smallIcon := loadIconFromFile(iconPath, 32, 32)
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), Style: CS_HREDRAW | CS_VREDRAW, LpfnWndProc: embeddedWndProcCallback, HInstance: syscall.Handle(hInst), HIcon: bigIcon, HIconSm: smallIcon, HbrBackground: syscall.Handle(6), LpszClassName: className}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16("SleepySource"))), uintptr(wsOverlappedWindow|WS_VISIBLE), uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 1450, 920, 0, 0, hInst, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("SleepySource could not create its desktop window: %v", err)
	}
	procShowWindow.Call(hwnd, swShowNormal)
	procUpdateWindow.Call(hwnd)
	// Tray failure is non-fatal. When available, clicking X hides SleepySource
	// to the tray so the webhook relay can keep its current public URL.
	_ = installSystemTray(app, syscall.Handle(hwnd))
	return syscall.Handle(hwnd), nil
}
func runEmbeddedDesigner(app *App) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := procCoInitializeExWV.Call(0, coinitApartmentThreaded)
	if hresultFailed(hr) {
		return fmt.Errorf("SleepySource could not initialize the Windows WebView2 apartment (HRESULT 0x%08X).", uint32(hr))
	}
	defer procCoUninitializeWV.Call()
	runtimePath := findEmbeddedBrowserWebViewDLL()
	if runtimePath == "" {
		return fmt.Errorf("SleepySource could not find the Microsoft WebView2 Runtime.\n\nInstall or repair Microsoft Edge WebView2 Runtime, then open SleepySource again.")
	}
	k32 := syscall.NewLazyDLL("kernel32.dll")
	loadLibraryExW := k32.NewProc("LoadLibraryExW")
	getProcAddress := k32.NewProc("GetProcAddress")
	freeLibrary := k32.NewProc("FreeLibrary")
	runtimePtr := syscall.StringToUTF16Ptr(runtimePath)
	const loadWithAlteredSearchPath = 0x00000008
	dllHandleRaw, _, loadErr := loadLibraryExW.Call(uintptr(unsafe.Pointer(runtimePtr)), 0, loadWithAlteredSearchPath)
	if dllHandleRaw == 0 {
		return fmt.Errorf("SleepySource found WebView2 but could not load it:\n%s\n\n%v", runtimePath, loadErr)
	}
	dllHandle := syscall.Handle(dllHandleRaw)
	defer freeLibrary.Call(uintptr(dllHandle))
	procName := append([]byte("CreateWebViewEnvironmentWithOptionsInternal"), 0)
	createProc, _, _ := getProcAddress.Call(uintptr(dllHandle), uintptr(unsafe.Pointer(&procName[0])))
	if createProc == 0 {
		return fmt.Errorf("Your WebView2 Runtime does not expose the embedded environment API SleepySource needs.\n\nUpdate or repair Microsoft Edge WebView2 Runtime.")
	}
	hwnd, err := createEmbeddedWindow(app)
	if err != nil {
		return err
	}
	host := &embeddedDesignerHost{hwnd: hwnd}
	activeEmbeddedDesigner = host
	defer func() { activeEmbeddedDesigner = nil }()
	host.envHandler = newWVEnvHandler(host)
	host.envOptions = newWVEnvOptions()
	dataDir := filepath.Join(app.dataDir, "DesktopRuntime")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		procDestroyWindow.Call(uintptr(hwnd))
		return fmt.Errorf("SleepySource could not create its WebView2 data folder: %w", err)
	}
	userData := syscall.StringToUTF16Ptr(dataDir)
	// Prevent external environment/registry overrides from changing this instance.
	_ = os.Setenv("WEBVIEW2_PIPE_FOR_SCRIPT_DEBUGGER", "")
	_ = os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "")
	_ = os.Setenv("WEBVIEW2_RELEASE_CHANNEL_PREFERENCE", "0")
	_ = os.Setenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER", "")
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", "")
	// Signature used by Microsoft's installed Evergreen client DLL: unknown=1,
	// runtime type=installed, user-data folder, environment options, completion handler.
	hr, _, _ = syscall.SyscallN(createProc, 1, webViewRuntimeInstalled, uintptr(unsafe.Pointer(userData)), uintptr(unsafe.Pointer(host.envOptions)), uintptr(unsafe.Pointer(host.envHandler)))
	if hresultFailed(hr) {
		procDestroyWindow.Call(uintptr(hwnd))
		return fmt.Errorf("WebView2 could not start the embedded SleepySource Designer (HRESULT 0x%08X).", uint32(hr))
	}

	// Never leave the user staring at a blank window indefinitely again.
	startupTimer := time.AfterFunc(12*time.Second, func() {
		if !host.ready.Load() {
			host.setError(fmt.Errorf("WebView2 started but did not attach the Designer within 12 seconds.\n\nTry closing SleepySource, deleting SleepySource_Data\\DesktopRuntime, and opening it again."))
			procPostMessageWVW.Call(uintptr(hwnd), designerFatalCloseMessage, 0, 0)
		}
	})
	defer startupTimer.Stop()
	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if host.core != nil {
		wvCall(host.core.vtbl.Release, uintptr(unsafe.Pointer(host.core)))
	}
	if host.controller != nil {
		wvCall(host.controller.vtbl.Close, uintptr(unsafe.Pointer(host.controller)))
		wvCall(host.controller.vtbl.Release, uintptr(unsafe.Pointer(host.controller)))
	}
	if host.env != nil {
		wvCall(host.env.vtbl.Release, uintptr(unsafe.Pointer(host.env)))
	}
	if err := host.getError(); err != nil {
		return err
	}
	return nil
}
