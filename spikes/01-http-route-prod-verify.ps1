<#
SPIKE (fix-wave follow-up to Task 2, §5 item 1) — verification script, kept
for auditability.

Not production code. Task 2's original finding (spikes/01-http-route-finding.md)
only exercised the custom `/page/` route through `wails3 dev`, where the
AssetServer proxies `/` to the Vite dev server (`AssetFileServerFS` used only
as a fallback path). Production builds skip the Vite proxy entirely and serve
`frontend/dist` straight from the embedded `embed.FS` via the same
`AssetFileServerFS`. The whole-branch review flagged that this path was never
actually exercised — Task 2's finding says production "seria esperado"
(expected) to work, based on code inspection, not measurement.

This script closes that gap: it launches the actual PRODUCTION binary
(`spikes/wails-poc/bin/wails-poc.exe`, built via `wails3 build`, no dev
server involved) and uses Windows UI Automation (System.Windows.Automation)
to click the real "testar rota custom" link inside the real native window,
then reads back the rendered text — the same technique Task 2 and Task 5
used against `wails3 dev` windows, now pointed at the embedded-assets
production binary instead.

How to re-run:
  1. cd spikes/wails-poc
  2. wails3 build            (produces bin/wails-poc.exe fresh)
  3. powershell -NoProfile -ExecutionPolicy Bypass -File ../01-http-route-prod-verify.ps1
#>

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

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

$exePath = Join-Path $PSScriptRoot "wails-poc\bin\wails-poc.exe"
if (-not (Test-Path $exePath)) {
    throw "production binary not found at $exePath -- run 'wails3 build' in spikes/wails-poc first"
}
Write-Host "=== Launching PRODUCTION binary: $exePath ==="

$proc = Start-Process -FilePath $exePath -PassThru
Write-Host "PID: $($proc.Id)"

$root = [System.Windows.Automation.AutomationElement]::RootElement
$cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ProcessIdProperty, $proc.Id)

$deadline = (Get-Date).AddSeconds(30)
$win = $null
while ((Get-Date) -lt $deadline) {
    $windows = $root.FindAll([System.Windows.Automation.TreeScope]::Children, $cond)
    if ($windows.Count -gt 0) {
        $win = $windows[0]
        break
    }
    Start-Sleep -Milliseconds 500
}
if (-not $win) {
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    throw "no top-level window appeared for PID $($proc.Id) within 30s"
}
Write-Host "Found window: $($win.Current.Name)"

Start-Sleep -Seconds 2

Write-Host "`n=== Locating 'testar rota custom' link ==="
$deadline2 = (Get-Date).AddSeconds(10)
$link = $null
while ((Get-Date) -lt $deadline2) {
    $link = Find-ByName $win 'testar rota custom' $null
    if ($link) { break }
    Start-Sleep -Milliseconds 300
}
if (-not $link) {
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    throw "link 'testar rota custom' not found in production window within 10s"
}
Write-Host "Link found. Invoking via InvokePattern (production embedded-assets path, no Vite dev server involved)."

$linkPattern = $link.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
$linkPattern.Invoke()
Start-Sleep -Seconds 1

Write-Host "`n=== Reading rendered text after navigation ==="
$typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
$found = $false
$deadline3 = (Get-Date).AddSeconds(10)
while ((Get-Date) -lt $deadline3 -and -not $found) {
    $texts = $win.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
    foreach ($t in $texts) {
        if ($t.Current.Name -like 'spike ok:*') {
            Write-Host "PRODUCTION ROUTE OK: found text node '$($t.Current.Name)'"
            $found = $true
        }
    }
    if (-not $found) { Start-Sleep -Milliseconds 300 }
}
if (-not $found) {
    Write-Host "PRODUCTION ROUTE FAIL: no 'spike ok:' text node found after clicking the link"
}

Write-Host "`n=== Cleanup ==="
Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
Write-Host "Killed PID $($proc.Id)."

Write-Host "`n=== DONE ==="
if (-not $found) { exit 1 }
exit 0
