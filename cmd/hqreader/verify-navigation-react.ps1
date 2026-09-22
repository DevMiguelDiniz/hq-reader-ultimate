<#
Task 5 (Etapa 3b) — manual E2E verification script, kept for auditability.

Not production code. Throwaway PowerShell using Windows UI Automation
(System.Windows.Automation) to drive the REAL native window opened by
hqreader.exe (either `wails3 dev`'s bin/hqreader.exe, or the `wails3 build`
production bin/hqreader.exe — same UI in both cases, different asset-serving
code path underneath: Vite dev-proxy vs. embedded AssetFileServerFS).

This is an ADAPTATION of the Etapa 2 script `verify-navigation.ps1` (kept
committed alongside this one for history), updated for the React frontend
landed in Tasks 1-4 of this plan. The old script targeted the vanilla-JS
app's hand-rolled DOM (`<img id="page">`, `<span id="counter">`, `<button
id="prev">`/`<button id="next">`). The new app renders the nav buttons as
shadcn/ui `<Button variant="secondary">` components, which are still real
`<button>` elements whose accessible Name is exactly "Anterior" and
"Proxima" (see cmd/hqreader/frontend/src/components/Reader.tsx) — so the
UI-Automation-by-name button lookup/invoke logic from the old script needed
NO changes there.

The counter DID need a real change, found only by inspecting the live UI
Automation tree of the running React app (not guessed — a `Dump-Tree`-style
descendant walk was run against the dev process before writing this
script). The Etapa 2 counter was `<span id="counter">1 / 3</span>`, a
single DOM text node, exposed as ONE UI Automation Text control whose Name
was the full "1 / 3" string. The React counter is JSX:
`<span>{currentPage + 1} / {pageCount}</span>` — and JSX renders each
`{expression}` and each literal string chunk as ITS OWN separate DOM text
node. The live tree confirms this: instead of one Text node, WebView2
exposes three sibling Text controls in document order — Name='1',
Name=' / ', Name='3' — with no id and no single node containing the full
"N / M" string. The old script's per-node regex match
(`$el.Current.Name -match '^\d+\s*/\s*\d+$'`) therefore NEVER matches
against the new DOM and would silently return $null forever. Get-CounterText
below is rewritten to collect ALL Text descendants in document order and
concatenate their Name values before applying the same regex to the joined
string — this is the one substantive adaptation this script makes beyond a
find-and-replace of comments.

What actually changed from the old script, and why:
  - Get-CounterText rewritten to concatenate sibling Text node Names (see
    above) instead of regex-matching a single node's Name.
  - Two new checks (part of Steps C/F below) read `IsEnabled` directly off
    the "Proxima"/"Anterior" AutomationElement via Get-ButtonEnabled, to
    confirm the real HTML `disabled` attribute (shadcn/Radix Button,
    reflected as IsEnabled=False in UI Automation) is set at the range
    limits — not just that the counter stops changing. The live tree
    confirmed this reflects correctly: right after initial load (page 1/3)
    the 'Anterior' button already showed IsEnabled=False in the raw
    inspection dump.
  - Header/doc comments updated to describe the React app and the shadcn
    Button/store-backed navigation (Reader.tsx calls useReaderStore's
    goToPage(), which calls the real SetCurrentPage backend method on every
    click — this script's PASS/FAIL checks are therefore indirectly
    exercising SetCurrentPage/Current on the Go side, not just client-side
    state).
  - Step G (image-presence check): the live tree shows the image node as
    `[ControlType.Image] Name='pagina'` (matching Reader.tsx's
    `alt="pagina"`), same as Etapa 2's `<img id="page" alt="pagina">` — the
    ControlType.Image existence check is unchanged.
  - Everything else (window resolution by PID, restore/foreground dance,
    retry loop shape, PASS/FAIL bookkeeping, exit codes) is copied as-is
    from the Etapa 2 script, because none of it is specific to the old
    vanilla DOM.

What it checks, against a running hqreader.exe process whose window shows
the reader UI (React <img alt="pagina">, <span>N / M</span> counter,
"Anterior"/"Proxima" shadcn buttons) for a book opened via HQREADER_BOOK:
  A. Initial counter text -> expect "1 / 3".
  B. Click "Proxima" -> expect "2 / 3".
  C. Click "Proxima" again -> expect "3 / 3", and the "Proxima" button
     itself must report IsEnabled=False (disabled at the last page — this
     is the real HTML `disabled` attribute set by
     `disabled={currentPage >= pageCount - 1}` in Reader.tsx, surfaced by
     UI Automation, not just inferred from index math).
  D. Click "Proxima" a THIRD time while disabled (best-effort Invoke on a
     disabled control) -> expect the counter to STAY "3 / 3" (index must
     not advance past pageCount-1; SetCurrentPage must not be called with
     an out-of-range index, or if it is, Current()/state must still clamp).
  E. Click "Anterior" -> expect "2 / 3".
  F. Click "Anterior" twice more -> expect "1 / 3", and the "Anterior"
     button itself must report IsEnabled=False (disabled at the first page,
     `disabled={currentPage <= 0}` in Reader.tsx).
  G. Confirm the <img alt="pagina"> element is present in the accessibility
     tree (Image control), as a secondary check that the reader viewport
     actually rendered an image element (not just the loading/error
     fallback text that Reader.tsx renders instead when `loading` or
     `error`/`!bookID` is true).

Known WebView2 UI Automation pitfall (found while writing the Etapa 2
script, still applies here unchanged): if the window is started
minimized/hidden (e.g. because the launcher process used -WindowStyle
Hidden), WebView2 exposes ZERO descendants via UI Automation until the
window is restored and brought to the foreground. This script defensively
restores+foregrounds the window before reading anything, so it is not
sensitive to how the caller launched the process.

How to re-run (dev mode):
  1. cd cmd/hqreader
  2. $env:HQREADER_BOOK = "<path to a 3-page .cbz>"
  3. wails3 dev   (in the background; wait for the "hqreader" native window)
  4. powershell -NoProfile -ExecutionPolicy Bypass -File verify-navigation-react.ps1 -ProcessName hqreader

How to re-run (production mode):
  1. cd cmd/hqreader
  2. wails3 build
  3. $env:HQREADER_BOOK = "<path to a 3-page .cbz>"
  4. Start-Process .\bin\hqreader.exe
  5. powershell -NoProfile -ExecutionPolicy Bypass -File verify-navigation-react.ps1 -ProcessName hqreader

Exits 0 if every check passes, 1 otherwise.
#>

param(
    [string]$ProcessName = "hqreader"
)

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class HqReaderVerifyReactWin32 {
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
}
"@

function Find-ByName($parent, $name, $controlType) {
    $nameCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::NameProperty, $name)
    if ($controlType) {
        $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, $controlType)
        $cond = New-Object System.Windows.Automation.AndCondition($nameCond, $typeCond)
    } else {
        $cond = $nameCond
    }
    return $parent.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $cond)
}

function Invoke-ByName($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    $pattern = $btn.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
    $pattern.Invoke()
}

function Get-ButtonEnabled($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    return $btn.Current.IsEnabled
}

function Get-CounterText($parent) {
    # The counter is React JSX: <span>{currentPage + 1} / {pageCount}</span>
    # (no id — see Reader.tsx). Unlike the old vanilla <span id="counter">
    # (which was ONE DOM text node, exposed as ONE UI Automation Text
    # control named e.g. "1 / 3"), JSX renders each {expression} and each
    # literal string chunk as its OWN separate DOM text node. Confirmed by
    # inspecting the live tree of the running app: three sibling Text
    # controls in document order, Name='1', Name=' / ', Name='3' — no
    # single node contains the full "N / M" string. So instead of
    # regex-matching one node's Name, concatenate ALL Text descendants'
    # Name values (in the order FindAll returns them, which is document
    # order) and match the regex against the joined string. Retry briefly:
    # the accessibility tree can take a moment to expose freshly-rendered
    # nodes after a click/re-render.
    $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
    $deadline = (Get-Date).AddSeconds(10)
    while ((Get-Date) -lt $deadline) {
        $all = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
        $joined = ($all | ForEach-Object { $_.Current.Name }) -join ''
        $joined = $joined -replace '\s+', ' '
        $joined = $joined.Trim()
        if ($joined -match '^\d+\s*/\s*\d+$') { return $joined }
        Start-Sleep -Milliseconds 300
    }
    return $null
}

$global:pass = 0
$global:fail = 0
function Check($label, $actual, $expected) {
    if ($actual -eq $expected) {
        Write-Host "PASS: $label -> '$actual'"
        $global:pass++
    } else {
        Write-Host "FAIL: $label -> got '$actual', expected '$expected'"
        $global:fail++
    }
}

function CheckBool($label, $actual, $expected) {
    if ($actual -eq $expected) {
        Write-Host "PASS: $label -> $actual"
        $global:pass++
    } else {
        Write-Host "FAIL: $label -> got $actual, expected $expected"
        $global:fail++
    }
}

Write-Host "=== Locating $ProcessName.exe process ==="
$proc = Get-Process $ProcessName -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $proc) { throw "$ProcessName.exe not running" }
Write-Host "PID: $($proc.Id)"

$root = [System.Windows.Automation.AutomationElement]::RootElement
$cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ProcessIdProperty, $proc.Id)

$deadline = (Get-Date).AddSeconds(30)
$win = $null
while ((Get-Date) -lt $deadline) {
    $windows = $root.FindAll([System.Windows.Automation.TreeScope]::Children, $cond)
    if ($windows.Count -gt 0) { $win = $windows[0]; break }
    Start-Sleep -Milliseconds 500
}
if (-not $win) { throw "no top-level window appeared for PID $($proc.Id) within 30s" }
Write-Host "Found window: $($win.Current.Name)"

Write-Host "`n=== Restoring/foregrounding window (WebView2 exposes no UI Automation descendants while minimized/hidden) ==="
if ($proc.MainWindowHandle -ne [IntPtr]::Zero) {
    [HqReaderVerifyReactWin32]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null   # SW_RESTORE
    [HqReaderVerifyReactWin32]::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
}
Start-Sleep -Milliseconds 800
Write-Host "BoundingRectangle after restore: $($win.Current.BoundingRectangle)"

Write-Host "`n=== Step A: initial counter (expect '1 / 3') ==="
$c0 = Get-CounterText $win
Write-Host "Initial counter text: '$c0'"
Check "Initial counter" $c0 "1 / 3"

Write-Host "`n=== Step B: click Proxima -> expect '2 / 3' ==="
Invoke-ByName $win 'Proxima'
Start-Sleep -Milliseconds 500
$c1 = Get-CounterText $win
Check "After 1x Proxima" $c1 "2 / 3"

Write-Host "`n=== Step C: click Proxima again -> expect '3 / 3', and Proxima becomes disabled ==="
Invoke-ByName $win 'Proxima'
Start-Sleep -Milliseconds 500
$c2 = Get-CounterText $win
Check "After 2x Proxima" $c2 "3 / 3"
$proximaEnabledAtEnd = Get-ButtonEnabled $win 'Proxima'
CheckBool "Proxima button IsEnabled at last page (expect False)" $proximaEnabledAtEnd $false

Write-Host "`n=== Step D: click Proxima a 3rd time (disabled, past end) -> expect STILL '3 / 3' (clamped) ==="
try {
    Invoke-ByName $win 'Proxima'
} catch {
    Write-Host "Invoke on disabled 'Proxima' raised (expected/acceptable): $_"
}
Start-Sleep -Milliseconds 500
$c3 = Get-CounterText $win
Check "After 3x Proxima (clamp at end)" $c3 "3 / 3"

Write-Host "`n=== Step E: click Anterior -> expect '2 / 3' ==="
Invoke-ByName $win 'Anterior'
Start-Sleep -Milliseconds 500
$c4 = Get-CounterText $win
Check "After 1x Anterior" $c4 "2 / 3"

Write-Host "`n=== Step F: click Anterior twice more -> expect '1 / 3', Anterior becomes disabled, then stays '1 / 3' (clamped at start) ==="
Invoke-ByName $win 'Anterior'
Start-Sleep -Milliseconds 500
$c5 = Get-CounterText $win
Check "After 2x Anterior" $c5 "1 / 3"
$anteriorEnabledAtStart = Get-ButtonEnabled $win 'Anterior'
CheckBool "Anterior button IsEnabled at first page (expect False)" $anteriorEnabledAtStart $false

try {
    Invoke-ByName $win 'Anterior'
} catch {
    Write-Host "Invoke on disabled 'Anterior' raised (expected/acceptable): $_"
}
Start-Sleep -Milliseconds 500
$c6 = Get-CounterText $win
Check "After 3x Anterior (clamp at start)" $c6 "1 / 3"

Write-Host "`n=== Step G: confirm <img alt='pagina'> is present in the accessibility tree ==="
$imgCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Image)
$imgs = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, $imgCond)
Write-Host "Image nodes found: $($imgs.Count)"
foreach ($i in $imgs) { Write-Host "  Image Name='$($i.Current.Name)'" }
if ($imgs.Count -gt 0) {
    Write-Host "PASS: page <img> element present in accessibility tree"
    $global:pass++
} else {
    Write-Host "FAIL: no <img> element found in accessibility tree"
    $global:fail++
}

Write-Host "`n=== SUMMARY: $global:pass passed, $global:fail failed ==="
if ($global:fail -gt 0) { exit 1 }
exit 0
