<#
SPIKE (Task 5, §5 item 4) — verification script, kept for auditability.

Not production code. Throwaway PowerShell using Windows UI Automation
(System.Windows.Automation) to drive the two REAL native windows opened by
`wails3 dev` (spikes/wails-poc), because the Browser-pane tools only see a
separate Chromium instance and cannot see/drive the native WebView2 windows
Wails opens (see spikes/01-http-route-finding.md for the same constraint
found in Task 2).

What it checks, against the running `wails-poc.exe` process:
  A. Initial (pre-click) counter text in both windows.
  B. Click "incrementar" 3x in Window 1 -> read counter -> expect "3".
  C. Read Window 2's counter BEFORE it does anything -> expect stale "0"
     (no push-based sync between windows, only Get() on demand).
  D. Click "incrementar" ONCE in Window 2 (its handler calls Increment()
     then Get()) -> expect "4" (3 from Window 1 + 1 from Window 2), NOT "1".
     This is the load-bearing check: "4" only happens if both windows share
     the same backend CounterService instance.
  E. Close Window 2 via WindowPattern.Close() (UI Automation's programmatic
     equivalent of clicking the native close button - goes through the real
     window-close path, not a process kill).
  F. Verify: `wails-poc.exe` process still alive with the SAME PID, and
     Window 1 still functional (one more click -> expect "5").
  G. Regression check for Task 2's spike: click "testar rota custom" in
     Window 1 and confirm the custom /page/ route still renders
     "spike ok: /page/test-123".

How to re-run:
  1. cd spikes/wails-poc
  2. wails3 dev -config ./build/config.yml -port 9245   (in the background;
     wait for both "Window 1" and "Window 2" native windows to appear)
  3. powershell -NoProfile -ExecutionPolicy Bypass -File ../04-multiwindow-verify.ps1
  4. Kill wails-poc.exe and the node/Vite processes it spawned afterwards.

Raw output from the run this spike's finding file cites is pasted verbatim
into spikes/04-multiwindow-finding.md (captured the same way, via
`Start-Transcript`/redirected stdout of this exact script).
#>

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

function Get-TopWindowsForProcess($procId) {
    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ProcessIdProperty, $procId)
    return $root.FindAll([System.Windows.Automation.TreeScope]::Children, $cond)
}

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

function Find-NumericTextNodes($parent) {
    $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
    $all = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
    $results = @()
    foreach ($el in $all) {
        $n = $el.Current.Name
        if ($n -match '^[0-9]+$') { $results += $n }
    }
    return $results
}

function Invoke-ByName($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    $pattern = $btn.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
    $pattern.Invoke()
}

Write-Host "=== Locating wails-poc.exe process ==="
$proc = Get-Process wails-poc -ErrorAction SilentlyContinue
if (-not $proc) { throw "wails-poc.exe not running" }
Write-Host "PID (before): $($proc.Id)"

# Poll until both windows are visible (dev server startup + WebView2 init can take a few seconds)
$deadline = (Get-Date).AddSeconds(30)
$windows = @()
while ((Get-Date) -lt $deadline) {
    $windows = Get-TopWindowsForProcess $proc.Id
    $names = $windows | ForEach-Object { $_.Current.Name }
    Write-Host "Windows seen so far: $($names -join ', ')"
    if (($names | Where-Object { $_ -eq 'Window 1' }) -and ($names | Where-Object { $_ -eq 'Window 2' })) {
        break
    }
    Start-Sleep -Seconds 1
}

$win1 = $windows | Where-Object { $_.Current.Name -eq 'Window 1' } | Select-Object -First 1
$win2 = $windows | Where-Object { $_.Current.Name -eq 'Window 2' } | Select-Object -First 1
if (-not $win1 -or -not $win2) { throw "did not find both Window 1 and Window 2" }
Write-Host "Found both windows."

Start-Sleep -Seconds 2

function Wait-ForNumericValue($parent, $label) {
    # The counter <span> renders "0" on load, but the WebView2/Chromium
    # accessibility tree can take a moment to expose it as a UI Automation
    # Text node after the page first paints. Retry briefly instead of
    # reporting a false "no value yet" (which happened on some earlier runs
    # of this script when a single one-shot read raced the initial render).
    $deadline2 = (Get-Date).AddSeconds(5)
    while ((Get-Date) -lt $deadline2) {
        $vals = Find-NumericTextNodes $parent
        if ($vals.Count -gt 0) { return $vals }
        Start-Sleep -Milliseconds 300
    }
    Write-Host "WARNING: no numeric text node found for $label within 5s"
    return @()
}

Write-Host "`n=== Step A: initial values (before any click) ==="
$w1Initial = Wait-ForNumericValue $win1 'Window 1 (initial)'
Write-Host "Window 1 numeric text nodes (initial): $($w1Initial -join ',')"
$w2Initial = Wait-ForNumericValue $win2 'Window 2 (initial)'
Write-Host "Window 2 numeric text nodes (initial): $($w2Initial -join ',')"

Write-Host "`n=== Step B: click 'incrementar' 3x in Window 1 ==="
for ($i = 0; $i -lt 3; $i++) {
    Invoke-ByName $win1 'incrementar'
    Start-Sleep -Milliseconds 400
}
Start-Sleep -Milliseconds 500
$w1After = Find-NumericTextNodes $win1
Write-Host "Window 1 numeric text nodes after 3 clicks: $($w1After -join ',')"
if ($w1Initial.Count -gt 0 -and $w1After.Count -gt 0) {
    Write-Host ("Delta (Window 1, 3 clicks): {0} -> {1} (diff={2}, expected diff=3)" -f @($w1Initial)[0], @($w1After)[0], ([int]@($w1After)[0] - [int]@($w1Initial)[0]))
}

Write-Host "`n=== Step C: read Window 2 BEFORE it does anything (should still show its stale initial value) ==="
$w2Before = Find-NumericTextNodes $win2
Write-Host "Window 2 numeric text nodes (stale, pre-refresh): $($w2Before -join ',')"

Write-Host "`n=== Step D: click 'incrementar' once in Window 2 (calls Increment()+Get() against the SAME backend) ==="
Invoke-ByName $win2 'incrementar'
Start-Sleep -Milliseconds 500
$w2After = Find-NumericTextNodes $win2
Write-Host "Window 2 numeric text nodes after its own click: $($w2After -join ',')"
if ($w1After.Count -gt 0 -and $w2After.Count -gt 0) {
    $expected = [int]@($w1After)[0] + 1
    Write-Host ("EXPECTATION if state is shared: Window 2 shows {0} (Window 1's value {1} + Window 2's own +1 click), NOT {2} (which is what an ISOLATED counter starting from Window 2's own stale value {3} + 1 would show)." -f $expected, @($w1After)[0], ([int]@($w2Before)[0] + 1), @($w2Before)[0])
}

Write-Host "`n=== Step E: close Window 2 via WindowPattern.Close() (equivalent to OS close button) ==="
$winPattern2 = $win2.GetCurrentPattern([System.Windows.Automation.WindowPattern]::Pattern)
$winPattern2.Close()
Start-Sleep -Seconds 2

Write-Host "`n=== Step F: verify process still alive and Window 1 still functional ==="
$procAfter = Get-Process wails-poc -ErrorAction SilentlyContinue
if ($procAfter) {
    Write-Host "Process wails-poc.exe STILL RUNNING. PID (after)=$($procAfter.Id) HasExited=$($procAfter.HasExited)"
} else {
    Write-Host "Process wails-poc.exe IS NO LONGER RUNNING (app died)."
}

$windowsAfter = Get-TopWindowsForProcess $proc.Id
$namesAfter = $windowsAfter | ForEach-Object { $_.Current.Name }
Write-Host "Remaining top-level windows: $($namesAfter -join ', ')"

$win1Still = $windowsAfter | Where-Object { $_.Current.Name -eq 'Window 1' } | Select-Object -First 1
if ($win1Still) {
    Invoke-ByName $win1Still 'incrementar'
    Start-Sleep -Milliseconds 500
    $w1Final = Find-NumericTextNodes $win1Still
    Write-Host "Window 1 numeric text nodes after closing Window 2 + one more click: $($w1Final -join ',')"
    if ($w2After.Count -gt 0 -and $w1Final.Count -gt 0) {
        Write-Host ("EXPECTATION: {0} (Window 2's last value {1} + 1), proving Window 1's backend/state survived Window 2's close." -f ([int]@($w2After)[0] + 1), @($w2After)[0])
    }
} else {
    Write-Host "Window 1 element handle no longer resolvable (unexpected)."
}

Write-Host "`n=== Step G: regression check - Task 2's custom /page/ route still works in Window 1 ==="
if ($win1Still) {
    $link = Find-ByName $win1Still 'testar rota custom' $null
    if (-not $link) {
        Write-Host "REGRESSION FAIL: link 'testar rota custom' not found"
    } else {
        $linkPattern = $link.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
        $linkPattern.Invoke()
        Start-Sleep -Seconds 1
        $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
        $texts = $win1Still.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
        $found = $false
        foreach ($t in $texts) {
            if ($t.Current.Name -like 'spike ok:*') {
                Write-Host "REGRESSION OK: found text node '$($t.Current.Name)'"
                $found = $true
            }
        }
        if (-not $found) { Write-Host "REGRESSION FAIL: no 'spike ok:' text node found after clicking the link" }
    }
}

Write-Host "`n=== DONE ==="
