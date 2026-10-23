<#
SPIKE (Etapa 4, Task 1) - script de verificacao descartavel, mantido para auditoria.

Dirige a janela REAL do spike06 (aberta por `wails3 dev` ou pelo exe) com
entrada real de mouse/teclado:
  1. (-Cancel) clique real em "Escolher pasta", espera ~1 s, lista as janelas
     do processo (o dialogo nativo), envia {ESC} por SendKeys, le o resultado;
  2. (-Select) abre o dialogo por InvokePattern e aperta o OK do proprio
     dialogo (AutomationId "1"); o app abre o dialogo em SPIKE_PICK_DIR;
  3. Burst / Sample / Paralelo (InvokePattern; o clique nao importa aqui);
  4. fecha a janela por WindowPattern.Close() e espera o processo sair
     (ServiceShutdown deve aparecer no log).
A evidencia principal e o log em SPIKE_LOG (o frontend reporta tudo via
SpikeService.Report); este script imprime o que ve na UI.

Uso: powershell -NoProfile -ExecutionPolicy Bypass -File verify-ui.ps1 -AppPid <pid de spike06.exe> [-Cancel] [-Select]
#>
param(
    [Parameter(Mandatory = $true)][int]$AppPid,
    [switch]$Cancel,
    [switch]$Select
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms

Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class Spike06Win32 {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowText(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetClassName(IntPtr h, StringBuilder s, int n);
    [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
    [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, int dwData, UIntPtr dwExtraInfo);
    public static string Describe(IntPtr h) {
        StringBuilder t = new StringBuilder(512); GetWindowText(h, t, 512);
        StringBuilder c = new StringBuilder(256); GetClassName(h, c, 256);
        uint pid; GetWindowThreadProcessId(h, out pid);
        return String.Format("hwnd=0x{0:X} pid={1} class='{2}' title='{3}'", h.ToInt64(), pid, c, t);
    }
}
"@
[Spike06Win32]::SetProcessDPIAware() | Out-Null

$UIA = [System.Windows.Automation.AutomationElement]
$Scope = [System.Windows.Automation.TreeScope]
$CT = [System.Windows.Automation.ControlType]

function Get-ProcWindows {
    $cond = New-Object System.Windows.Automation.PropertyCondition($UIA::ProcessIdProperty, $AppPid)
    return @($UIA::RootElement.FindAll($Scope::Children, $cond))
}

function Find-ByName($parent, $name, $controlType) {
    $c = New-Object System.Windows.Automation.PropertyCondition($UIA::NameProperty, $name)
    if ($controlType) {
        $t = New-Object System.Windows.Automation.PropertyCondition($UIA::ControlTypeProperty, $controlType)
        $c = New-Object System.Windows.Automation.AndCondition($c, $t)
    }
    return $parent.FindFirst($Scope::Descendants, $c)
}

function Read-Out($win, $prefix) {
    $t = New-Object System.Windows.Automation.PropertyCondition($UIA::ControlTypeProperty, $CT::Text)
    foreach ($el in $win.FindAll($Scope::Descendants, $t)) {
        $n = $el.Current.Name
        if ($n -like "$prefix*") { return $n }
    }
    return "<no text node starting with '$prefix'>"
}

function Wait-Out($win, $prefix, $notLike, $seconds) {
    $deadline = (Get-Date).AddSeconds($seconds)
    while ((Get-Date) -lt $deadline) {
        $v = Read-Out $win $prefix
        if ($v -notlike $notLike -and $v -notlike '<no text*') { return $v }
        Start-Sleep -Milliseconds 250
    }
    return (Read-Out $win $prefix)
}

function Click-At([double]$x, [double]$y) {
    $p = [System.Windows.Forms.Cursor]::Position
    for ($i = 1; $i -le 10; $i++) {
        [Spike06Win32]::SetCursorPos([int]($p.X + ($x - $p.X) * $i / 10), [int]($p.Y + ($y - $p.Y) * $i / 10)) | Out-Null
        Start-Sleep -Milliseconds 20
    }
    Start-Sleep -Milliseconds 80
    [Spike06Win32]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 60
    [Spike06Win32]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
}

function RealClick-Button($win, $name) {
    $b = Find-ByName $win $name $CT::Button
    if (-not $b) { throw "button '$name' not found" }
    $r = $b.Current.BoundingRectangle
    Write-Host ("real click on '{0}' at ({1},{2})" -f $name, [int]($r.X + $r.Width / 2), [int]($r.Y + $r.Height / 2))
    Click-At ($r.X + $r.Width / 2) ($r.Y + $r.Height / 2)
}

function Invoke-Button($win, $name) {
    $b = Find-ByName $win $name $CT::Button
    if (-not $b) { throw "button '$name' not found" }
    $b.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke()
}

function Show-ProcWindows($label) {
    Write-Host "-- windows of pid $AppPid ($label):"
    foreach ($w in Get-ProcWindows) {
        Write-Host ("   name='{0}' class='{1}' hwnd=0x{2:X}" -f $w.Current.Name, $w.Current.ClassName, $w.Current.NativeWindowHandle)
    }
    Write-Host ("-- foreground: " + [Spike06Win32]::Describe([Spike06Win32]::GetForegroundWindow()))
}

Write-Host "=== locating window 'spike06' of pid $AppPid ==="
$win = $null
$deadline = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $deadline -and -not $win) {
    $win = Get-ProcWindows | Where-Object { $_.Current.Name -eq 'spike06' } | Select-Object -First 1
    if (-not $win) { Start-Sleep -Milliseconds 500 }
}
if (-not $win) { throw "window spike06 not found" }
[Spike06Win32]::SetForegroundWindow([IntPtr]$win.Current.NativeWindowHandle) | Out-Null
Write-Host ("startup text: " + (Wait-Out $win 'startup-ping' '*aguardando*' 10))

if ($Cancel) {
    Write-Host "`n=== 1. PickFolder + ESC (real mouse click) ==="
    RealClick-Button $win 'Escolher pasta'
    Start-Sleep -Milliseconds 1000
    Show-ProcWindows 'about 1 s after the click'
    [System.Windows.Forms.SendKeys]::SendWait('{ESC}')
    Write-Host "sent {ESC}"
    Write-Host ("pick text: " + (Wait-Out $win 'pick' 'pick: nada' 10))
    Show-ProcWindows 'after ESC'
}

if ($Select) {
    # Sem mouse: o campo "Pasta:" do dialogo de pasta nao aparece como Edit na
    # arvore UIA (dump real: so celulas da lista e a caixa de pesquisa). O app
    # abre o dialogo ja em SPIKE_PICK_DIR (opcao Directory) e aqui so se aperta o
    # botao OK do proprio dialogo (AutomationId "1"), que devolve a pasta atual.
    Write-Host "`n=== 2. PickFolder + OK (dialog opened in SPIKE_PICK_DIR) ==="
    $before = Read-Out $win 'pick'
    Invoke-Button $win 'Escolher pasta'
    $dlg = $null
    $deadline = (Get-Date).AddSeconds(10)
    while ((Get-Date) -lt $deadline -and -not $dlg) {
        $dlg = Get-ProcWindows | Where-Object { $_.Current.ClassName -eq '#32770' } | Select-Object -First 1
        if (-not $dlg) { Start-Sleep -Milliseconds 250 }
    }
    if (-not $dlg) { throw "dialog (#32770) not found" }
    Start-Sleep -Milliseconds 1000
    Write-Host ("dialog: name='{0}'" -f $dlg.Current.Name)
    $okCond = New-Object System.Windows.Automation.PropertyCondition($UIA::AutomationIdProperty, '1')
    $ok = $dlg.FindFirst($Scope::Descendants, $okCond)
    # O botao "Selecionar pasta" aparece como Pane sem nenhum padrao UIA (sem
    # InvokePattern): aperta-se via WM_COMMAND(IDOK) para a janela do dialogo.
    Write-Host ("OK button: name='{0}' type={1}; posting WM_COMMAND IDOK" -f $ok.Current.Name, $ok.Current.ControlType.ProgrammaticName)
    [Spike06Win32]::PostMessage([IntPtr]$dlg.Current.NativeWindowHandle, 0x0111, [IntPtr]1, [IntPtr]::Zero) | Out-Null
    Write-Host ("pick text: " + (Wait-Out $win 'pick' $before 10))
}

Write-Host "`n=== 3. Burst ==="
Invoke-Button $win 'Burst'
Write-Host ("burst text: " + (Wait-Out $win 'burst' 'burst: nada' 30))

Write-Host "`n=== 4. Sample ==="
Invoke-Button $win 'Sample'
Write-Host ("sample text: " + (Wait-Out $win 'sample' 'sample: nada' 10))

Write-Host "`n=== 5. Paralelo ==="
Invoke-Button $win 'Paralelo'
Write-Host ("par text: " + (Wait-Out $win 'par' 'par: nada' 10))

Write-Host "`n=== 6. close window ==="
$win.GetCurrentPattern([System.Windows.Automation.WindowPattern]::Pattern).Close()
$p = Get-Process -Id $AppPid -ErrorAction SilentlyContinue
if ($p) { $exited = $p.WaitForExit(15000); Write-Host "process $AppPid exited within 15 s: $exited" } else { Write-Host "process $AppPid already gone" }
Write-Host "=== DONE ==="
