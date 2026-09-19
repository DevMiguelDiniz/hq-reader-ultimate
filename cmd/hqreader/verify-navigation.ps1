<#
Task 6 (Etapa 2) — manual E2E verification script, kept for auditability.

Not production code. Throwaway PowerShell using Windows UI Automation
(System.Windows.Automation) to drive the REAL native window opened by
hqreader.exe (either `wails3 dev`'s bin/hqreader.exe, or the `wails3 build`
production bin/hqreader.exe — same UI in both cases, different asset-serving
code path underneath: Vite dev-proxy vs. embedded AssetFileServerFS). Same
technique already validated in Etapa 0 — see spikes/04-multiwindow-verify.ps1
and spikes/01-http-route-prod-verify.ps1 for the established pattern this
script matches: locate the real window by PID via UI Automation, click real
buttons via InvokePattern, read real rendered text back via the
accessibility tree, and print PASS/FAIL per step with the actual value
observed, not just a description of the expected behaviour.

What it checks, against a running hqreader.exe process whose window shows
the reader UI (<img id="page">, <span id="counter">, "Anterior"/"Proxima"
buttons) for a book opened via HQREADER_BOOK:
  A. Initial counter text -> expect "1 / 3".
  B. Click "Proxima" -> expect "2 / 3".
  C. Click "Proxima" again -> expect "3 / 3".
  D. Click "Proxima" a THIRD time (attempting to go past the last page) ->
     expect the counter to STAY "3 / 3" (index must not advance past
     pageCount-1).
  E. Click "Anterior" -> expect "2 / 3".
  F. Click "Anterior" twice more -> expect "1 / 3", and one more "Anterior"
     click after that must NOT go below "1 / 3" (index must not regress
     below 0).
  G. Confirm the <img id="page" alt="pagina"> element is present in the
     accessibility tree (Image control), as a secondary check that the
     reader viewport actually rendered an image element (not just the
     error-fallback text).

Known WebView2 UI Automation pitfall (found while writing this script): if
the window is started minimized/hidden (e.g. because the launcher process
used -WindowStyle Hidden), WebView2 exposes ZERO descendants via UI
Automation until the window is restored and brought to the foreground. This
script defensively restores+foregrounds the window before reading anything,
so it is not sensitive to how the caller launched the process.

How to re-run (dev mode):
  1. cd cmd/hqreader
  2. $env:HQREADER_BOOK = "<path to a 3-page .cbz>"
  3. wails3 dev   (in the background; wait for the "hqreader" native window)
  4. powershell -NoProfile -ExecutionPolicy Bypass -File verify-navigation.ps1 -ProcessName hqreader

How to re-run (production mode):
  1. cd cmd/hqreader
  2. wails3 build
  3. $env:HQREADER_BOOK = "<path to a 3-page .cbz>"
  4. Start-Process .\bin\hqreader.exe
  5. powershell -NoProfile -ExecutionPolicy Bypass -File verify-navigation.ps1 -ProcessName hqreader

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
public class HqReaderVerifyWin32 {
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

function Get-CounterText($parent) {
    # The counter is a <span id="counter"> rendering text like "1 / 3".
    # WebView2/Chromium exposes it as a UI Automation Text node whose Name
    # matches that pattern. Retry briefly: the accessibility tree can take a
    # moment to expose a freshly-rendered node after a click.
    $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
    $deadline = (Get-Date).AddSeconds(10)
    while ((Get-Date) -lt $deadline) {
        $all = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
        foreach ($el in $all) {
            if ($el.Current.Name -match '^\d+\s*/\s*\d+$') { return $el.Current.Name }
        }
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
    [HqReaderVerifyWin32]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null   # SW_RESTORE
    [HqReaderVerifyWin32]::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
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

Write-Host "`n=== Step C: click Proxima again -> expect '3 / 3' ==="
Invoke-ByName $win 'Proxima'
Start-Sleep -Milliseconds 500
$c2 = Get-CounterText $win
Check "After 2x Proxima" $c2 "3 / 3"

Write-Host "`n=== Step D: click Proxima a 3rd time (past end) -> expect STILL '3 / 3' (clamped) ==="
Invoke-ByName $win 'Proxima'
Start-Sleep -Milliseconds 500
$c3 = Get-CounterText $win
Check "After 3x Proxima (clamp at end)" $c3 "3 / 3"

Write-Host "`n=== Step E: click Anterior -> expect '2 / 3' ==="
Invoke-ByName $win 'Anterior'
Start-Sleep -Milliseconds 500
$c4 = Get-CounterText $win
Check "After 1x Anterior" $c4 "2 / 3"

Write-Host "`n=== Step F: click Anterior twice more -> expect '1 / 3', then stays '1 / 3' (clamped at start) ==="
Invoke-ByName $win 'Anterior'
Start-Sleep -Milliseconds 500
$c5 = Get-CounterText $win
Check "After 2x Anterior" $c5 "1 / 3"

Invoke-ByName $win 'Anterior'
Start-Sleep -Milliseconds 500
$c6 = Get-CounterText $win
Check "After 3x Anterior (clamp at start)" $c6 "1 / 3"

Write-Host "`n=== Step G: confirm <img id='page' alt='pagina'> is present in the accessibility tree ==="
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
