<#
Etapa 3c — verificacao E2E das interacoes do leitor (zoom, pan, modo de
pagina unica/dupla, atalhos de teclado, placeholder de erro por pagina,
reset ao trocar de pagina, redimensionar com zoom ativo).

Nao e codigo de producao. PowerShell descartavel que dirige a janela REAL do
hqreader.exe com Windows UI Automation (System.Windows.Automation), com
entrada real de mouse/teclado (user32 SetCursorPos/mouse_event e
System.Windows.Forms.SendKeys) e com capturas de pixel da janela (user32
PrintWindow). Roda igual contra `wails3 dev` e contra o binario de
`wails3 build` (bin/hqreader.exe).

NAO MEXA NO MOUSE NEM NO TECLADO enquanto o script roda: ele move o cursor
de verdade (inclusive ate as bordas da tela nos arrastos grandes) e envia
teclas para a janela em primeiro plano.

Pre-requisito: o livro aberto e o CBZ de 10 paginas gerado por
`go run make-verify-cbz.go <saida.cbz>` (de cmd/hqreader), apontado por
HQREADER_BOOK. Indices 0-based: 0 capa, 4 spread (paisagem), 6 corrompida.
Visoes no modo duplo: [0] [1,2] [3] [4] [5,6] [7,8] [9]. Toda pagina nao
corrompida e uma cor solida com uma FAIXA MARCADORA branca pura
(255,255,255), largura total, 40 px de altura, centralizada na vertical
(linhas 430..469 das paginas de 900 px), e uma faixa vertical magenta pura
(255,0,255), altura total, 40 px de largura, centralizada na horizontal
(colunas 280..319 das paginas de 600 px). A branca mede o pan vertical, a
magenta o horizontal.

O que o script le na arvore de UI Automation (cada dado e UM no de texto;
confirmado num dump real da arvore antes de escrever o script):
  - contador:  Text com Name "N / M" ou "N-M / M"
  - zoom:      Text com Name "NNN%"
  - modo:      botao com Name "Pagina unica" ou "Pagina dupla" (o rotulo e o
               modo ATUAL)
  - paginas:   controles Image com Name "pagina N" (1-based)
  - erro:      botao "Tentar novamente" e um Text contendo "carregar esta"
  - viewport:  BoundingRectangle do Document (= area cliente do WebView) e
               do botao "Proxima" (limites do viewport, ver Get-Viewport)

O que o script NAO le da arvore: a geometria das paginas. O BoundingRectangle
do Image e o retangulo de LAYOUT e ignora o `transform` CSS de zoom/pan (no
dump real: 'pagina 1' Rect=783;259;354;529 tanto a 100% quanto a 135%, com a
pagina visivelmente maior na captura). Toda a geometria (arrastar moveu a
pagina, pan limitado, arrasto sem efeito quando a pagina cabe, pagina visivel
depois de maximizar/restaurar, zoom ancorado no cursor) e medida em pixels
de uma captura PrintWindow da janela: Get-MarkerStats acha os pixels brancos
da faixa marcadora (todos os canais >= 250; o fundo e rgb(23,23,23) e as
cores das paginas nunca sao brancas), os pixels magenta (R,B >= 250, G <= 5;
nenhuma cor de pagina tem R=255) e os pixels de pagina (diferentes do
fundo), dentro da regiao do viewport.

Fatos medidos no app real que as contas abaixo usam:
  - um "notch" da roda (WHEEL_DELTA 120) chega ao WebView como deltaY=100;
    o PageView aplica zoom*exp(-deltaY*0.0015), entao cada notch multiplica
    o zoom por exp(0.15) = 1.1618 (1 notch 116%, 2 notches 135%, 3 notches
    157%, 4 notches 182%; 14 notches satura em 400%, 10 para baixo em 50%).
  - teclado: + e = multiplicam por 1.15, - divide por 1.15, 0 volta a 100%.

Como rodar (dev):
  1. cd cmd/hqreader
  2. go run make-verify-cbz.go $env:TEMP\hqreader-interactions.cbz
  3. $env:HQREADER_BOOK = "$env:TEMP\hqreader-interactions.cbz"
  4. $p = Start-Process wails3 -ArgumentList dev -PassThru   (espere a janela
     "hqreader"; se o wails3 dev nao conectar ao Vite na primeira vez, pare a
     arvore dele e rode de novo)
  5. .\verify-reader-interactions.ps1 *> verify-reader-interactions-dev-output.txt
     (ou: powershell -NoProfile -ExecutionPolicy Bypass -File verify-reader-interactions.ps1)
  6. taskkill /T /F /PID $p.Id   (pare SO a arvore que voce iniciou)

Como rodar (producao): wails3 build; mesmo HQREADER_BOOK;
$p = Start-Process .\bin\hqreader.exe -PassThru; mesmo comando do passo 5;
taskkill /T /F /PID $p.Id.

Precisa haver exatamente UM processo hqreader.exe (ou passe -ProcessId).
Sai com 0 se todas as checagens passam, 1 caso contrario.
#>

param(
    [string]$ProcessName = "hqreader",
    [int]$ProcessId = 0
)

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class HqReaderInteractionsWin32 {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, int dwData, UIntPtr dwExtraInfo);
}
"@

# Captura PrintWindow(PW_RENDERFULLCONTENT) da janela inteira e analisa os
# pixels com LockBits (um loop de pixels em PowerShell puro seria lento
# demais). As coordenadas de entrada e de saida sao de TELA: o bitmap tem o
# tamanho de GetWindowRect e o pixel (0,0) do bitmap e o canto
# (Left,Top) de GetWindowRect (no dump real o Image 'pagina 1' em tela
# x=783,y=259 com a janela em 510,190 apareceu no bitmap em x=273..274,y=69).
Add-Type -ReferencedAssemblies System.Drawing -TypeDefinition @"
using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
public class HqReaderMarkerStats {
    public int Count;            // pixels brancos (faixa marcadora)
    public double MeanY;         // Y medio dos pixels brancos (tela)
    public int MinX, MaxX, MinY, MaxY;          // extensao dos pixels brancos
    public int MCount;           // pixels magenta (faixa vertical)
    public double MMeanX;        // X medio dos pixels magenta (tela)
    public int MMinX, MMaxX;
    public int PageCount;        // pixels de pagina (nao-fundo)
    public int PageMinX, PageMaxX, PageMinY, PageMaxY;
    public int RegionLeft, RegionTop, RegionRight, RegionBottom;
    public override string ToString() {
        System.Globalization.CultureInfo ic = System.Globalization.CultureInfo.InvariantCulture;
        string w = Count == 0 ? "marker count=0" : String.Format(ic, "marker count={0} meanY={1:0.0} y={2}..{3} x={4}..{5}", Count, MeanY, MinY, MaxY, MinX, MaxX);
        string m = MCount == 0 ? "vmarker count=0" : String.Format(ic, "vmarker count={0} meanX={1:0.0} x={2}..{3}", MCount, MMeanX, MMinX, MMaxX);
        return String.Format(ic, "{0}; {1}; page y={2}..{3} x={4}..{5}; region y={6}..{7} x={8}..{9}",
            w, m, PageMinY, PageMaxY, PageMinX, PageMaxX, RegionTop, RegionBottom, RegionLeft, RegionRight);
    }
}
public class HqReaderCapture {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] static extern bool GetWindowRect(IntPtr hwnd, out RECT r);
    [DllImport("user32.dll")] static extern bool PrintWindow(IntPtr hwnd, IntPtr hdc, uint flags);

    public static RECT WindowRect(IntPtr hwnd) { RECT r; GetWindowRect(hwnd, out r); return r; }

    // Regiao [left,right) x [top,bottom) em coordenadas de tela.
    public static HqReaderMarkerStats Measure(IntPtr hwnd, int left, int top, int right, int bottom) {
        RECT r; GetWindowRect(hwnd, out r);
        int w = r.Right - r.Left, h = r.Bottom - r.Top;
        HqReaderMarkerStats s = new HqReaderMarkerStats();
        s.MinX = s.MinY = s.PageMinX = s.PageMinY = s.MMinX = int.MaxValue;
        s.MaxX = s.MaxY = s.PageMaxX = s.PageMaxY = s.MMaxX = int.MinValue;
        int x0 = Math.Max(0, left - r.Left), x1 = Math.Min(w, right - r.Left);
        int y0 = Math.Max(0, top - r.Top), y1 = Math.Min(h, bottom - r.Top);
        s.RegionLeft = x0 + r.Left; s.RegionRight = x1 + r.Left - 1;
        s.RegionTop = y0 + r.Top; s.RegionBottom = y1 + r.Top - 1;
        using (Bitmap bmp = new Bitmap(w, h, PixelFormat.Format32bppArgb)) {
            using (Graphics g = Graphics.FromImage(bmp)) {
                IntPtr hdc = g.GetHdc();
                PrintWindow(hwnd, hdc, 2); // PW_RENDERFULLCONTENT
                g.ReleaseHdc(hdc);
            }
            BitmapData d = bmp.LockBits(new Rectangle(0, 0, w, h), ImageLockMode.ReadOnly, PixelFormat.Format32bppArgb);
            byte[] buf = new byte[d.Stride * h];
            Marshal.Copy(d.Scan0, buf, 0, buf.Length);
            int stride = d.Stride;
            bmp.UnlockBits(d);
            double sumY = 0, sumMX = 0;
            for (int y = y0; y < y1; y++) {
                int row = y * stride;
                for (int x = x0; x < x1; x++) {
                    int i = row + x * 4;
                    int b = buf[i], gg = buf[i + 1], rr = buf[i + 2];
                    int sx = x + r.Left, sy = y + r.Top;
                    if (rr >= 250 && gg >= 250 && b >= 250) {
                        s.Count++; sumY += sy;
                        if (sx < s.MinX) s.MinX = sx; if (sx > s.MaxX) s.MaxX = sx;
                        if (sy < s.MinY) s.MinY = sy; if (sy > s.MaxY) s.MaxY = sy;
                    }
                    // faixa vertical magenta (255,0,255): nenhuma cor de pagina tem R=255
                    if (rr >= 250 && gg <= 5 && b >= 250) {
                        s.MCount++; sumMX += sx;
                        if (sx < s.MMinX) s.MMinX = sx; if (sx > s.MMaxX) s.MMaxX = sx;
                    }
                    // fundo do app: rgb(23,23,23) (bg-neutral-900)
                    if (Math.Abs(rr - 23) > 12 || Math.Abs(gg - 23) > 12 || Math.Abs(b - 23) > 12) {
                        s.PageCount++;
                        if (sx < s.PageMinX) s.PageMinX = sx; if (sx > s.PageMaxX) s.PageMaxX = sx;
                        if (sy < s.PageMinY) s.PageMinY = sy; if (sy > s.PageMaxY) s.PageMaxY = sy;
                    }
                }
            }
            if (s.Count > 0) { s.MeanY = sumY / s.Count; }
            if (s.MCount > 0) { s.MMeanX = sumMX / s.MCount; }
        }
        return s;
    }
}
"@

# Coordenadas fisicas para UI Automation, SetCursorPos e GetWindowRect
# (no monitor principal a 100% isso nao muda nada; evita virtualizacao de DPI
# se a escala for outra).
[HqReaderInteractionsWin32]::SetProcessDPIAware() | Out-Null

$MOUSEEVENTF_LEFTDOWN = 0x0002
$MOUSEEVENTF_LEFTUP = 0x0004
$MOUSEEVENTF_WHEEL = 0x0800

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

function CheckTrue($label, $condition, $detail) {
    if ($condition) {
        Write-Host "PASS: $label ($detail)"
        $global:pass++
    } else {
        Write-Host "FAIL: $label ($detail)"
        $global:fail++
    }
}

function R1($v) { return [math]::Round([double]$v, 1) }

# ---- UI Automation helpers ------------------------------------------------

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

function Test-ButtonExists($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    return [bool]$btn
}

# Espera (ate 5s) o botao aparecer ($expected=$true) ou sumir ($false): a UI
# assenta de forma assincrona (store, carga da imagem, fade de saida do
# Motion, que mantem a visao antiga montada por ~75ms).
function Wait-ButtonPresence($parent, $name, $expected) {
    $deadline = (Get-Date).AddSeconds(5)
    $present = Test-ButtonExists $parent $name
    while ($present -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $present = Test-ButtonExists $parent $name
    }
    return $present
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

function Get-TextNames($parent) {
    $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
    $all = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
    $names = @()
    foreach ($el in $all) { $names += $el.Current.Name }
    return $names
}

function Get-TextMatching($parent, $pattern) {
    $deadline = (Get-Date).AddSeconds(10)
    while ((Get-Date) -lt $deadline) {
        foreach ($n in (Get-TextNames $parent)) {
            if ($n -match $pattern) { return $n }
        }
        Start-Sleep -Milliseconds 300
    }
    return $null
}

function Get-Counter($parent) { return Get-TextMatching $parent '^\d+(-\d+)? / \d+$' }

function Get-ZoomPercent($parent) {
    $t = Get-TextMatching $parent '^\d+%$'
    if ($t) { return [int]($t -replace '%', '') }
    return $null
}

function Get-ImageNames($parent) {
    $cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Image)
    $imgs = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $cond)
    $names = @()
    foreach ($i in $imgs) { $names += $i.Current.Name }
    return (($names | Sort-Object) -join ',')
}

# Polling checks: the UI settles asynchronously (store update, image load,
# fade), so wait up to 5s for the expected value before declaring failure.
function Check-Counter($label, $expected) {
    $deadline = (Get-Date).AddSeconds(5)
    $actual = Get-Counter $script:win
    while ($actual -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $actual = Get-Counter $script:win
    }
    Check $label $actual $expected
}

function Check-Zoom($label, $expected) {
    $deadline = (Get-Date).AddSeconds(5)
    $actual = Get-ZoomPercent $script:win
    while ($actual -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $actual = Get-ZoomPercent $script:win
    }
    Check $label $actual $expected
}

function Check-Images($label, $expectedNames) {
    $expected = (($expectedNames | Sort-Object) -join ',')
    $deadline = (Get-Date).AddSeconds(5)
    $actual = Get-ImageNames $script:win
    while ($actual -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $actual = Get-ImageNames $script:win
    }
    Check $label $actual $expected
}

function Check-ButtonPresent($label, $name) {
    $present = Wait-ButtonPresence $script:win $name $true
    CheckTrue $label $present "button '$name' present=$present"
}

function Check-ButtonAbsent($label, $name) {
    $present = Wait-ButtonPresence $script:win $name $false
    CheckTrue $label (-not $present) "button '$name' present=$present"
}

# ---- Pixel geometry helpers -------------------------------------------------

# Limites do viewport do PageView em coordenadas de tela, tirados da arvore:
#  - topo: o Document do WebView ocupa a area cliente inteira e o viewport e
#    o primeiro filho do layout em coluna (dump real: Document
#    Rect=518;221;884;661 com a janela em 510,190,900x700).
#  - base: a linha de controles e `py-3` (12 px) em volta de botoes `h-8`
#    (32 px), os itens mais altos da linha; logo o viewport termina 12 px
#    acima do topo do botao "Proxima" (dump real: Proxima top=838, base do
#    Document=882 -> linha de controles 826..882, 56 px; viewport 221..826).
function Get-Viewport() {
    $docCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Document)
    $doc = $script:win.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $docCond)
    if (-not $doc) { throw "Document element not found" }
    $btn = Find-ByName $script:win 'Proxima' ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button 'Proxima' not found" }
    $d = $doc.Current.BoundingRectangle
    $b = $btn.Current.BoundingRectangle
    $top = [double]$d.Y
    $bottom = [double]$b.Y - 12
    return @{
        Left = [double]$d.X; Right = [double]($d.X + $d.Width)
        Width = [double]$d.Width; CenterX = [double]($d.X + $d.Width / 2)
        DocHeight = [double]$d.Height
        Top = $top; Bottom = $bottom
        Height = $bottom - $top; CenterY = ($top + $bottom) / 2
    }
}

# Regiao analisada = so o viewport das paginas:
#  - em cima, 40 px abaixo do topo da janela: na captura real a barra de
#    titulo + moldura ocupam as linhas 0..30 (area cliente comeca na linha
#    31), entao 40 deixa margem e nunca inclui a barra de titulo;
#  - embaixo, 2 px acima da base do viewport (Get-Viewport), fora da linha de
#    controles (cujo texto e quase branco, rgb(245,245,245));
#  - nas laterais, 8 px para dentro de GetWindowRect (bordas invisiveis do
#    Windows 11: janela 510..1410, area cliente 518..1402).
function Measure-Once() {
    $hwnd = $script:proc.MainWindowHandle
    $wr = [HqReaderCapture]::WindowRect($hwnd)
    $vp = Get-Viewport
    return [HqReaderCapture]::Measure($hwnd, $wr.Left + 8, $wr.Top + 40, $wr.Right - 8, [int]($vp.Bottom - 2))
}

# Captura e mede; se a faixa ainda nao aparece (logo depois de trocar de
# pagina o fade de entrada do Motion deixa a faixa translucida, abaixo de 250
# em cada canal), tenta de novo por ate 3s. So espera pela PRESENCA da faixa;
# a posicao medida e a da primeira captura em que ela aparece.
function Get-MarkerStats() {
    $deadline = (Get-Date).AddSeconds(3)
    $s = Measure-Once
    while ($s.Count -eq 0 -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 200
        $s = Measure-Once
    }
    return $s
}

# Pan zero = pagina centralizada: a faixa branca (centro vertical da pagina)
# no centro vertical do viewport e a faixa magenta (centro horizontal) no
# centro horizontal, cada uma com tolerancia de 2 px.
function Check-PageCentred($label) {
    $vpc = Get-Viewport
    $st = Get-MarkerStats
    $ok = ($st.Count -gt 0) -and ($st.MCount -gt 0) -and ([math]::Abs($st.MeanY - $vpc.CenterY) -le 2) -and ([math]::Abs($st.MMeanX - $vpc.CenterX) -le 2)
    CheckTrue $label $ok "marker meanY=$(R1 $st.MeanY) vs viewport centre Y=$(R1 $vpc.CenterY); vmarker meanX=$(R1 $st.MMeanX) vs viewport centre X=$(R1 $vpc.CenterX)"
}

# ---- Real input helpers ---------------------------------------------------

# Parametros TIPADOS de proposito: sem tipo, um argumento negativo escrito
# como `-3` ou `-600` chega como STRING ("-3"), e entao `"-3" -ge 0` da
# verdadeiro (a comparacao de strings ignora o hifen) e `"-600" * 2` vira
# "-600-600" (dry run real deste script).
function Move-Cursor([double]$x, [double]$y) {
    [HqReaderInteractionsWin32]::SetCursorPos([int]$x, [int]$y) | Out-Null
}

# Leva o cursor ate (x,y) em 10 passos a partir da posicao atual, como uma
# mao faria, em vez de teletransportar. Isto importa: o runtime do Wails
# (@wailsio/runtime drag.js) guarda uma "borda de redimensionamento" a cada
# mousemove (clientY < 5 => "n-resize", inclusive clientY negativo durante um
# arrasto capturado que sai da janela por cima) e so a limpa no proximo
# mousemove longe das bordas. Se o cursor salta de volta para a pagina e
# aperta sem nenhum mousemove no caminho, o Wails trata o clique como inicio
# de um redimensionamento nativo da janela, que engole o mouseup, e o
# PageView fica com o arrasto armado (a pagina passa a seguir o cursor sem
# botao). Isso e um defeito real do app, alcancavel por uma pessoa
# (redimensionar a janela pela borda superior, faixa de 5 px que pertence ao
# viewport do PageView) e esta reportado em task-7-report.md; este script nao
# o testa (falharia de proposito) e evita disparar o caminho sintetico.
function Move-CursorPath([double]$x, [double]$y) {
    $p = [System.Windows.Forms.Cursor]::Position
    $steps = 10
    for ($i = 1; $i -le $steps; $i++) {
        Move-Cursor ($p.X + ($x - $p.X) * $i / $steps) ($p.Y + ($y - $p.Y) * $i / $steps)
        Start-Sleep -Milliseconds 20
    }
}

# Move o cursor SEM botao de (x,y) por (dx,dy), em passos.
function Send-Hover([double]$x, [double]$y, [double]$dx, [double]$dy) {
    Move-CursorPath $x $y
    $steps = 10
    for ($i = 1; $i -le $steps; $i++) {
        Move-Cursor ($x + $dx * $i / $steps) ($y + $dy * $i / $steps)
        Start-Sleep -Milliseconds 30
    }
    Start-Sleep -Milliseconds 400
}

function Click-At([double]$x, [double]$y) {
    Move-CursorPath $x $y
    Start-Sleep -Milliseconds 80
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTDOWN, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 60
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTUP, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 400
}

function Click-Button($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    $r = $btn.Current.BoundingRectangle
    Click-At ($r.X + $r.Width / 2) ($r.Y + $r.Height / 2)
}

# notches > 0 = wheel up (zoom in), < 0 = wheel down (zoom out).
function Send-Wheel([double]$x, [double]$y, [int]$notches) {
    Move-CursorPath $x $y
    Start-Sleep -Milliseconds 80
    $delta = if ($notches -ge 0) { 120 } else { -120 }
    for ($i = 0; $i -lt [math]::Abs($notches); $i++) {
        [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_WHEEL, 0, 0, $delta, [UIntPtr]::Zero)
        Start-Sleep -Milliseconds 120
    }
    Start-Sleep -Milliseconds 300
}

function Send-Drag([double]$x, [double]$y, [double]$dx, [double]$dy) {
    Move-CursorPath $x $y
    Start-Sleep -Milliseconds 100
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTDOWN, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 100
    $steps = 10
    for ($i = 1; $i -le $steps; $i++) {
        Move-Cursor ($x + $dx * $i / $steps) ($y + $dy * $i / $steps)
        Start-Sleep -Milliseconds 30
    }
    Start-Sleep -Milliseconds 100
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTUP, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 400
}

function Send-Keys($keys) {
    [System.Windows.Forms.SendKeys]::SendWait($keys)
    Start-Sleep -Milliseconds 450
}

function Get-PageCenter() {
    $r = $script:win.Current.BoundingRectangle
    return @{ X = ($r.X + $r.Width / 2); Y = ($r.Y + $r.Height * 0.45) }
}

# ---- Locate the window ----------------------------------------------------

Write-Host "=== Locating $ProcessName.exe process ==="
if ($ProcessId -gt 0) {
    $proc = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
    if (-not $proc) { throw "process $ProcessId not running" }
} else {
    $procs = @(Get-Process $ProcessName -ErrorAction SilentlyContinue)
    if ($procs.Count -eq 0) { throw "$ProcessName.exe not running" }
    if ($procs.Count -gt 1) { throw "more than one $ProcessName.exe running (PIDs $(($procs | ForEach-Object { $_.Id }) -join ', ')); stop the extra ones or pass -ProcessId" }
    $proc = $procs[0]
}
Write-Host "PID: $($proc.Id)"
Write-Host "Path: $($proc.Path)"
# So informativo (nenhuma checagem depende disto): a cadeia de processos pais
# distingue uma execucao sob `wails3 dev` de uma do binario de producao
# iniciado direto do shell.
$chain = @()
$cur = Get-CimInstance Win32_Process -Filter "ProcessId=$($proc.Id)" -ErrorAction SilentlyContinue
for ($k = 0; $k -lt 4 -and $cur; $k++) {
    $parentId = $cur.ParentProcessId
    $parent = Get-CimInstance Win32_Process -Filter "ProcessId=$parentId" -ErrorAction SilentlyContinue
    if (-not $parent) { $chain += "$parentId (exited)"; break }
    $chain += "$parentId ($($parent.Name))"
    $cur = $parent
}
Write-Host "Parent chain: $($chain -join ' <- ')"

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
if ($proc.MainWindowHandle -eq [IntPtr]::Zero) { throw "PID $($proc.Id) has no main window handle" }
[HqReaderInteractionsWin32]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null   # SW_RESTORE
[HqReaderInteractionsWin32]::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
Start-Sleep -Milliseconds 800
Write-Host "BoundingRectangle after restore: $($win.Current.BoundingRectangle)"

$c = Get-PageCenter
Click-At $c.X $c.Y   # give the WebView keyboard focus (no pan happens at 100%)

# ---- 6.1 Initial state ----------------------------------------------------
Write-Host "`n=== 6.1: initial state ==="
Check-Counter "initial counter" "1 / 10"
Check-Zoom "initial zoom" 100
Check-ButtonPresent "mode button reads 'Pagina unica'" 'Pagina unica'
Check-Images "initial image" @('pagina 1')
$vp = Get-Viewport
Write-Host "Viewport (screen): top=$(R1 $vp.Top) bottom=$(R1 $vp.Bottom) height=$(R1 $vp.Height) centerY=$(R1 $vp.CenterY)"
$s = Get-MarkerStats
Write-Host "Capture at 100%: $s"
CheckTrue "marker band visible at 100%" ($s.Count -gt 0) "count=$($s.Count)"
$page100H = $s.PageMaxY - $s.PageMinY + 1
$pageCenter100 = ($s.PageMinY + $s.PageMaxY) / 2
CheckTrue "page centred vertically in the viewport at 100% (|page centre - viewport centre| <= 2)" ([math]::Abs($pageCenter100 - $vp.CenterY) -le 2) "page centre=$(R1 $pageCenter100) viewport centre=$(R1 $vp.CenterY) page height=$page100H"
CheckTrue "marker band sits at the page's vertical centre (|marker meanY - page centre| <= 2)" ([math]::Abs($s.MeanY - $pageCenter100) -le 2) "marker meanY=$(R1 $s.MeanY) page centre=$(R1 $pageCenter100)"
$page100W = $s.PageMaxX - $s.PageMinX + 1
$pageCenterX100 = ($s.PageMinX + $s.PageMaxX) / 2
CheckTrue "page centred horizontally in the viewport at 100% (|page centre X - viewport centre X| <= 2)" ([math]::Abs($pageCenterX100 - $vp.CenterX) -le 2) "page centre X=$(R1 $pageCenterX100) viewport centre X=$(R1 $vp.CenterX) page width=$page100W viewport width=$(R1 $vp.Width)"
CheckTrue "vertical (magenta) marker sits at the page's horizontal centre (|meanX - page centre X| <= 2)" (($s.MCount -gt 0) -and ([math]::Abs($s.MMeanX - $pageCenterX100) -le 2)) "vmarker meanX=$(R1 $s.MMeanX) count=$($s.MCount)"
# The page <img> is `max-h-[80vh]` (90vw is not the binding limit for a 2:3
# page), so its height at 100% is 80% of the document height. The maximize
# check in 6.5 relies on this rule to predict the page size in a viewport
# where it cannot be measured, so verify it here where it can.
CheckTrue "page height at 100% is 80% of the document height (max-h-[80vh]) (+-1.5 px)" ([math]::Abs($page100H - 0.8 * $vp.DocHeight) -le 1.5) "page height=$page100H 0.8*document height=$(R1 (0.8 * $vp.DocHeight))"

# ---- 6.2 Keyboard navigation (single mode) --------------------------------
Write-Host "`n=== 6.2: keyboard navigation, single mode ==="
Send-Keys "{RIGHT}"; Check-Counter "Right arrow" "2 / 10"
Send-Keys "{LEFT}";  Check-Counter "Left arrow" "1 / 10"
Send-Keys "d";       Check-Counter "key d" "2 / 10"
Send-Keys "a";       Check-Counter "key a" "1 / 10"
Send-Keys " ";       Check-Counter "Space" "2 / 10"
Send-Keys "{LEFT}";  Check-Counter "back to start" "1 / 10"
Send-Keys "{RIGHT 5}"; Check-Counter "burst of 5x Right" "6 / 10"
Check-Images "burst of 5x Right leaves exactly the page 6 image" @('pagina 6')
Send-Keys "{LEFT 5}";  Check-Counter "burst of 5x Left" "1 / 10"
Check-Images "burst of 5x Left leaves exactly the page 1 image" @('pagina 1')
Send-Keys "^{RIGHT}";  Check-Counter "Ctrl+Right must NOT navigate" "1 / 10"
Send-Keys "{LEFT}";    Check-Counter "Left at first page stays" "1 / 10"
Click-Button $win 'Proxima'
Check-Counter "real mouse click on Proxima" "2 / 10"
Send-Keys " "
Check-Counter "Space after clicking a button advances exactly one page" "3 / 10"
Start-Sleep -Milliseconds 1000
Check-Counter "still exactly one page after a 1s settle (no delayed second advance)" "3 / 10"
Send-Keys "{LEFT 2}"; Check-Counter "back to start again" "1 / 10"

# ---- 6.3 Keyboard zoom -----------------------------------------------------
Write-Host "`n=== 6.3: keyboard zoom ==="
Send-Keys "{+}"; Check-Zoom "key + once" 115
Send-Keys "-";   Check-Zoom "key - once" 100
Send-Keys "{+}{+}"; Check-Zoom "key + twice" 132
Send-Keys "0";   Check-Zoom "key 0 resets" 100
Send-Keys "=";   Check-Zoom "key = zooms in" 115
Send-Keys "0";   Check-Zoom "key 0 resets again" 100

# ---- 6.4 Wheel zoom, limits, zoom anchored at the cursor --------------------
Write-Host "`n=== 6.4: wheel zoom and limits ==="
$c = Get-PageCenter
Send-Wheel $c.X $c.Y 3
$z = Get-ZoomPercent $win
CheckTrue "3 notches up zoom in (130..200)" (($z -ge 130) -and ($z -le 200)) "zoom=$z%"
Send-Wheel $c.X $c.Y -3
$z = Get-ZoomPercent $win
CheckTrue "3 notches down return near 100 (95..105)" (($z -ge 95) -and ($z -le 105)) "zoom=$z%"
Send-Keys "0"
Send-Wheel $c.X $c.Y 14
Check-Zoom "14 notches up clamp at 400" 400
Send-Keys "0"
Send-Wheel $c.X $c.Y -10
Check-Zoom "10 notches down clamp at 50" 50
Send-Keys "0"
Check-Zoom "reset after limits" 100

Write-Host "--- wheel zoom keeps the point under the cursor fixed ---"
# Start at 132% via the keyboard (zoom around the centre, pan stays 0, the
# page already taller than the viewport), then 2 wheel notches with the
# cursor 200 px BELOW the marker (still on the page). Zooming by
# ratio = zoomAfter/zoomBefore around the cursor puts the marker at
# cursorY - 200*ratio (~70 px above where it was); a zoom around the
# viewport centre would leave it in place. The resulting pan (~-70 px) is
# inside the clamp at ~179% (~+-170 px). Starting from 100% instead does not
# work: the first notch (116%) can only pan +-5 px, so the anchor is clamped
# (dry run: marker at 501 instead of 488) — correct clampPan behaviour.
Send-Keys "{+}{+}"
Check-Zoom "keyboard zoom to 132% before the anchored wheel zoom" 132
$zBefore = Get-ZoomPercent $win
$s0 = Get-MarkerStats
$cursorY = [int]($s0.MeanY + 200)
Send-Wheel $c.X $cursorY 2
$zAfter = Get-ZoomPercent $win
$s1 = Get-MarkerStats
Write-Host "Capture before: $s0"
Write-Host "Capture after:  $s1"
$ratio = $zAfter / $zBefore
$expectedY = $cursorY - ($cursorY - $s0.MeanY) * $ratio
CheckTrue "2 notches up at the cursor: marker moves to cursorY - offset*ratio (+-3 px)" (($s1.Count -gt 0) -and ([math]::Abs($s1.MeanY - $expectedY) -le 3)) "zoom $zBefore% -> $zAfter% cursorY=$cursorY marker before=$(R1 $s0.MeanY) after=$(R1 $s1.MeanY) expected=$(R1 $expectedY)"
Send-Keys "0"
Check-Zoom "reset after anchored zoom" 100

# ---- 6.5 Pan, clamp, page-change reset, resize while zoomed ----------------
Write-Host "`n=== 6.5: pan, clamp, reset on page change, resize while zoomed ==="
$c = Get-PageCenter

Write-Host "--- drag does nothing while the page fits (100% and 50%) ---"
$sA = Get-MarkerStats
Send-Drag $c.X $c.Y 80 50
$sB = Get-MarkerStats
Write-Host "Capture before: $sA"
Write-Host "Capture after:  $sB"
$dy = $sB.MeanY - $sA.MeanY; $dx = $sB.MMeanX - $sA.MMeanX
CheckTrue "drag (+80,+50) at 100% does not move the page (|dx|<=2, |dy|<=2)" (($sB.Count -gt 0) -and ($sB.MCount -gt 0) -and ([math]::Abs($dy) -le 2) -and ([math]::Abs($dx) -le 2)) "dx=$(R1 $dx) dy=$(R1 $dy)"
Send-Wheel $c.X $c.Y -10
Check-Zoom "zoomed out to the minimum for the drag test" 50
Check-PageCentred "below 100% (at 50%) the page stays centred in the viewport"
$sA = Get-MarkerStats
Send-Drag $c.X $c.Y 80 50
$sB = Get-MarkerStats
Write-Host "Capture before: $sA"
Write-Host "Capture after:  $sB"
$dy = $sB.MeanY - $sA.MeanY; $dx = $sB.MMeanX - $sA.MMeanX
CheckTrue "drag (+80,+50) at 50% does not move the page (|dx|<=2, |dy|<=2)" (($sB.Count -gt 0) -and ($sB.MCount -gt 0) -and ([math]::Abs($dy) -le 2) -and ([math]::Abs($dx) -le 2)) "dx=$(R1 $dx) dy=$(R1 $dy)"
Send-Keys "0"
Check-Zoom "reset before the pan test" 100

Write-Host "--- pan while zoomed ---"
Send-Wheel $c.X $c.Y 4
$zoomed = Get-ZoomPercent $win
CheckTrue "zoomed in for pan test (>150)" ($zoomed -gt 150) "zoom=$zoomed%"
$vp = Get-Viewport
# clampPan: |pan.y| <= (content height * zoom - viewport height) / 2, and the
# marker is the content's vertical centre, so a clamped page puts the marker
# at viewport centre +- that maximum.
$maxPanY = ($page100H * $zoomed / 100 - $vp.Height) / 2
Write-Host "Expected vertical pan limit: +-$(R1 $maxPanY) px (page height at 100%=$page100H, zoom=$zoomed%, viewport height=$(R1 $vp.Height))"
$r0 = Get-MarkerStats
Write-Host "Capture before drag: $r0"
CheckTrue "marker visible after zooming in" ($r0.Count -gt 0) "count=$($r0.Count)"

Send-Drag $c.X $c.Y 0 50
$r1 = Get-MarkerStats
Write-Host "Capture after +50 drag: $r1"
$dy1 = $r1.MeanY - $r0.MeanY
CheckTrue "vertical drag +50 moves the page down ~50 (25..75)" (($r1.Count -gt 0) -and ($dy1 -ge 25) -and ($dy1 -le 75)) "deltaY=$(R1 $dy1)"

Send-Drag $c.X $c.Y 0 600
$r2 = Get-MarkerStats
Write-Host "Capture after +600 drag: $r2"
$dy2 = $r2.MeanY - $r0.MeanY
CheckTrue "huge vertical drag is clamped (total shift >50 and <500)" (($r2.Count -gt 0) -and ($dy2 -gt 50) -and ($dy2 -lt 500)) "deltaY=$(R1 $dy2)"
$expectedDown = $vp.CenterY + $maxPanY
CheckTrue "clamped at the computed limit: marker at viewport centre + limit (+-4 px)" ([math]::Abs($r2.MeanY - $expectedDown) -le 4) "marker meanY=$(R1 $r2.MeanY) expected=$(R1 $expectedDown)"
CheckTrue "marker still inside the viewport after the clamp" (($r2.MinY -ge $vp.Top) -and ($r2.MaxY -le $vp.Bottom)) "marker y=$($r2.MinY)..$($r2.MaxY) viewport=$(R1 $vp.Top)..$(R1 $vp.Bottom)"
Send-Drag $c.X $c.Y 0 200
$r3 = Get-MarkerStats
$dy3 = $r3.MeanY - $r2.MeanY
CheckTrue "a further drag down at the limit does not move the page (|dy|<=2)" ([math]::Abs($dy3) -le 2) "deltaY=$(R1 $dy3)"

Send-Drag $c.X $c.Y 0 -600
$r4 = Get-MarkerStats
Write-Host "Capture after -600 drag: $r4"
$expectedUp = $vp.CenterY - $maxPanY
CheckTrue "huge drag up is clamped at the opposite limit: marker at viewport centre - limit (+-4 px)" (($r4.Count -gt 0) -and ([math]::Abs($r4.MeanY - $expectedUp) -le 4)) "marker meanY=$(R1 $r4.MeanY) expected=$(R1 $expectedUp)"

# The -600 drag ended with the cursor above the window. Moving back over the
# page with NO button pressed must not move it (Send-Hover walks there in
# steps from the top of the screen, then 80 px down).
Send-Hover ($c.X - 150) ($c.Y - 40) 0 80
$r5 = Get-MarkerStats
Write-Host "Capture after a no-button hover: $r5"
CheckTrue "moving the mouse with no button pressed does not move the page (|dx|<=2, |dy|<=2)" (($r5.Count -gt 0) -and ([math]::Abs($r5.MeanY - $r4.MeanY) -le 2) -and ([math]::Abs($r5.MMeanX - $r4.MMeanX) -le 2)) "dy=$(R1 ($r5.MeanY - $r4.MeanY)) dx=$(R1 ($r5.MMeanX - $r4.MMeanX))"

Write-Host "--- horizontal drag while the zoomed page is narrower than the viewport ---"
# clampPan: |pan.x| <= max(0, (content width * zoom - viewport width) / 2).
$zoomedW = $page100W * $zoomed / 100
CheckTrue "at $zoomed% the page is narrower than the viewport, so the horizontal pan limit is 0" ($zoomedW -lt $vp.Width) "page width=$(R1 $zoomedW) viewport width=$(R1 $vp.Width)"
$rx0 = Get-MarkerStats
Send-Drag $c.X $c.Y 100 0
$rx1 = Get-MarkerStats
Write-Host "Capture before horizontal drag: $rx0"
Write-Host "Capture after horizontal drag:  $rx1"
$dxm = $rx1.MMeanX - $rx0.MMeanX
CheckTrue "horizontal drag +100 does not move the narrower page (|dx|<=2)" (($rx1.MCount -gt 0) -and ([math]::Abs($dxm) -le 2)) "vmarker meanX before=$(R1 $rx0.MMeanX) after=$(R1 $rx1.MMeanX)"

Write-Host "--- resize while zoomed: maximize ---"
# Expected position (first principles): the page <img> is max-h-[80vh]
# (checked in 6.1), so in any window its height at 100% is 0.8 * document
# height; the vertical pan limit is (that * zoom - viewport height) / 2; the
# pan (CSS px, here = screen px) is kept unless it exceeds the new limit, in
# which case setMetrics re-clamps it; the marker (page centre) sits at
# viewport centre + pan. Restoring applies the same rule to the small window.
$vpPre = Get-Viewport
$panPre = $rx1.MeanY - $vpPre.CenterY
Write-Host "Pan before maximize: $(R1 $panPre) px (marker $(R1 $rx1.MeanY) - viewport centre $(R1 $vpPre.CenterY))"
[HqReaderInteractionsWin32]::ShowWindow($proc.MainWindowHandle, 3) | Out-Null   # SW_MAXIMIZE
Start-Sleep -Milliseconds 1500
Write-Host "Window rect maximized: $($win.Current.BoundingRectangle)"
$vpM = Get-Viewport
Write-Host "Viewport maximized (screen): top=$(R1 $vpM.Top) bottom=$(R1 $vpM.Bottom) height=$(R1 $vpM.Height) centerY=$(R1 $vpM.CenterY) document height=$(R1 $vpM.DocHeight)"
$sm = Get-MarkerStats
Write-Host "Capture maximized: $sm"
CheckTrue "marker still visible after maximize" ($sm.Count -gt 0) "count=$($sm.Count)"
Check-Zoom "zoom unchanged by maximize" $zoomed
$limitM = (0.8 * $vpM.DocHeight * $zoomed / 100 - $vpM.Height) / 2
$panM = [math]::Max(-$limitM, [math]::Min($limitM, $panPre))
$expM = $vpM.CenterY + $panM
CheckTrue "maximized: marker at viewport centre + pan re-clamped to the new limit (+-4 px)" ([math]::Abs($sm.MeanY - $expM) -le 4) "limit=+-$(R1 $limitM) (0.8*$(R1 $vpM.DocHeight)*$zoomed% - $(R1 $vpM.Height))/2, pan=$(R1 $panM), expected=$(R1 $expM), marker meanY=$(R1 $sm.MeanY)"
[HqReaderInteractionsWin32]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null   # SW_RESTORE
Start-Sleep -Milliseconds 1500
Write-Host "Window rect restored: $($win.Current.BoundingRectangle)"
$vpR = Get-Viewport
$sr = Get-MarkerStats
Write-Host "Capture restored: $sr"
CheckTrue "marker still visible after restore" ($sr.Count -gt 0) "count=$($sr.Count)"
Check-Zoom "zoom unchanged by restore" $zoomed
$limitR = ($page100H * $zoomed / 100 - $vpR.Height) / 2
$panR = [math]::Max(-$limitR, [math]::Min($limitR, $panM))
$expR = $vpR.CenterY + $panR
CheckTrue "restored: marker at viewport centre + pan re-clamped to the smaller limit (+-4 px)" ([math]::Abs($sr.MeanY - $expR) -le 4) "limit=+-$(R1 $limitR), pan=$(R1 $panR), expected=$(R1 $expR), marker meanY=$(R1 $sr.MeanY)"

Write-Host "--- horizontal pan on a page wider than the viewport (400%) ---"
# The page edges never enter the viewport (that is what the clamp does), so
# horizontal position is read from the vertical magenta marker at the page's
# horizontal centre: meanX = viewport centre X + pan.x.
Send-Keys "0"
Check-Zoom "reset before the 400% test" 100
Send-Wheel $c.X $c.Y 14
Check-Zoom "wheel up to the 400% limit" 400
$vp = Get-Viewport
$maxPanX = ($page100W * 4 - $vp.Width) / 2
Write-Host "Expected horizontal pan limit: +-$(R1 $maxPanX) px (page width at 100%=$page100W, zoom=400%, viewport width=$(R1 $vp.Width))"
CheckTrue "at 400% the page is wider than the viewport (horizontal limit > 0)" ($maxPanX -gt 50) "page width=$(4 * $page100W) viewport width=$(R1 $vp.Width)"
$h0 = Get-MarkerStats
Write-Host "Capture at 400%: $h0"
CheckTrue "vertical marker at the viewport centre before the horizontal drags (|meanX - centre X| <= 2)" (($h0.MCount -gt 0) -and ([math]::Abs($h0.MMeanX - $vp.CenterX) -le 2)) "vmarker meanX=$(R1 $h0.MMeanX) viewport centre X=$(R1 $vp.CenterX)"
Send-Drag $c.X $c.Y 100 0
$h1 = Get-MarkerStats
Write-Host "Capture after +100 horizontal drag: $h1"
$hdx1 = $h1.MMeanX - $h0.MMeanX
CheckTrue "horizontal drag +100 moves the wider page right ~100 (75..125)" (($h1.MCount -gt 0) -and ($hdx1 -ge 75) -and ($hdx1 -le 125)) "deltaX=$(R1 $hdx1)"
Send-Drag $c.X $c.Y 600 0
$h2 = Get-MarkerStats
Write-Host "Capture after +600 horizontal drag: $h2"
$expRight = $vp.CenterX + $maxPanX
CheckTrue "huge drag right is clamped at the computed limit: vertical marker at viewport centre X + limit (+-4 px)" (($h2.MCount -gt 0) -and ([math]::Abs($h2.MMeanX - $expRight) -le 4)) "vmarker meanX=$(R1 $h2.MMeanX) expected=$(R1 $expRight)"
CheckTrue "vertical marker still inside the viewport after the clamp" (($h2.MMinX -ge $vp.Left) -and ($h2.MMaxX -le $vp.Right)) "vmarker x=$($h2.MMinX)..$($h2.MMaxX) viewport=$(R1 $vp.Left)..$(R1 $vp.Right)"
Send-Drag $c.X $c.Y 200 0
$h3 = Get-MarkerStats
CheckTrue "a further drag right at the limit does not move the page (|dx|<=2)" ([math]::Abs($h3.MMeanX - $h2.MMeanX) -le 2) "deltaX=$(R1 ($h3.MMeanX - $h2.MMeanX))"
Send-Drag $c.X $c.Y -900 0
$h4 = Get-MarkerStats
Write-Host "Capture after -900 horizontal drag: $h4"
$expLeft = $vp.CenterX - $maxPanX
CheckTrue "huge drag left is clamped at the opposite limit: vertical marker at viewport centre X - limit (+-4 px)" (($h4.MCount -gt 0) -and ([math]::Abs($h4.MMeanX - $expLeft) -le 4)) "vmarker meanX=$(R1 $h4.MMeanX) expected=$(R1 $expLeft)"

Write-Host "--- reset on page change ---"
Invoke-ByName $win 'Proxima'
Check-Counter "Proxima while zoomed" "2 / 10"
Check-Zoom "page change resets zoom to 100" 100
Check-PageCentred "page change resets the pan in both axes: new page centred"
Invoke-ByName $win 'Anterior'
Check-Counter "Anterior back to first page" "1 / 10"

Write-Host "--- arrow keys keep navigating while zoomed ---"
Send-Wheel $c.X $c.Y 4
$zk = Get-ZoomPercent $win
CheckTrue "zoomed in before the arrow key (>150)" ($zk -gt 150) "zoom=$zk%"
Send-Drag $c.X $c.Y 0 100
$sk = Get-MarkerStats
$vp = Get-Viewport
CheckTrue "page panned before the arrow key (marker > 50 px from the viewport centre)" (($sk.Count -gt 0) -and ([math]::Abs($sk.MeanY - $vp.CenterY) -gt 50)) "marker meanY=$(R1 $sk.MeanY) viewport centre=$(R1 $vp.CenterY)"
Send-Keys "{RIGHT}"
Check-Counter "real Right arrow while zoomed advances exactly one page" "2 / 10"
Check-Zoom "page change by arrow key resets zoom" 100
Check-PageCentred "page change by arrow key resets the pan: new page centred"
Send-Keys "{LEFT}"
Check-Counter "Left arrow back to first page" "1 / 10"

# ---- 6.6 Double page mode --------------------------------------------------
# The store ignores next/prev while a double view is pending (a page of the
# pair not measured yet) and AnimatePresence mode="wait" mounts the new
# view's images only after the old one's 75ms exit, so every key in double
# mode waits for the expected images (Check-Images) before the next key.
Write-Host "`n=== 6.6: double page mode ==="
Write-Host "--- toggling the mode resets zoom AND pan (single -> double) ---"
Send-Wheel $c.X $c.Y 4
$zt = Get-ZoomPercent $win
CheckTrue "zoomed in before toggling (>150)" ($zt -gt 150) "zoom=$zt%"
Send-Drag $c.X $c.Y 0 100
$st0 = Get-MarkerStats
$vp = Get-Viewport
CheckTrue "page panned before toggling (marker > 50 px from the viewport centre)" (($st0.Count -gt 0) -and ([math]::Abs($st0.MeanY - $vp.CenterY) -gt 50)) "marker meanY=$(R1 $st0.MeanY) viewport centre=$(R1 $vp.CenterY)"
Send-Keys "m"
Check-ButtonPresent "mode button reads 'Pagina dupla'" 'Pagina dupla'
Check-Zoom "toggling to double resets zoom" 100
Check-PageCentred "toggling to double resets the pan: cover centred"
Check-Counter "cover alone" "1 / 10"
Check-Images "cover alone images" @('pagina 1')
Send-Keys "{RIGHT}"; Check-Counter "pair 2-3" "2-3 / 10"
Check-Images "pair 2-3 images" @('pagina 2', 'pagina 3')
Send-Keys "{RIGHT}"; Check-Counter "page 4 alone (neighbor of spread)" "4 / 10"
Check-Images "page 4 images" @('pagina 4')
Send-Keys "{RIGHT}"; Check-Counter "page 5 alone (spread)" "5 / 10"
Check-Images "spread image" @('pagina 5')
Send-Keys "{RIGHT}"; Check-Counter "pair 6-7 (7 is corrupted)" "6-7 / 10"
Check-Images "only page 6 renders as image" @('pagina 6')
Check-ButtonPresent "page 7 shows the retry button" 'Tentar novamente'
Send-Keys "{RIGHT}"; Check-Counter "pair 8-9" "8-9 / 10"
Check-Images "pair 8-9 images" @('pagina 8', 'pagina 9')
Send-Keys "{RIGHT}"; Check-Counter "last page alone" "10 / 10"
Check-Images "last page image" @('pagina 10')
Send-Keys "{RIGHT}"; Check-Counter "Right at last stays" "10 / 10"
CheckTrue "Proxima disabled at the last view" (-not (Get-ButtonEnabled $win 'Proxima')) "IsEnabled=False"
Send-Keys "{LEFT}"; Check-Counter "back to pair 8-9" "8-9 / 10"
Check-Images "back to pair 8-9 images" @('pagina 8', 'pagina 9')
Send-Keys "{LEFT}"; Check-Counter "back to pair 6-7" "6-7 / 10"
Check-Images "back to pair 6-7 images" @('pagina 6')
Send-Keys "{LEFT}"; Check-Counter "back to spread" "5 / 10"
Check-Images "back to spread image" @('pagina 5')
Send-Keys "{LEFT}"; Check-Counter "back to page 4" "4 / 10"
Check-Images "back to page 4 image" @('pagina 4')
Send-Keys "{LEFT}"; Check-Counter "back to pair 2-3" "2-3 / 10"
Check-Images "back to pair 2-3 images" @('pagina 2', 'pagina 3')
Send-Keys "{LEFT}"; Check-Counter "back to cover" "1 / 10"
Check-Images "back to cover image" @('pagina 1')
CheckTrue "Anterior disabled at the first view" (-not (Get-ButtonEnabled $win 'Anterior')) "IsEnabled=False"

Write-Host "--- toggle back to single from a pair ---"
# Every page was visited above, so every proportion is known and no view is
# pending: the burst cannot be dropped by the pending guard (Decision 8).
Send-Keys "{RIGHT 5}"; Check-Counter "to pair 8-9 again" "8-9 / 10"
Check-Images "pair 8-9 images after the burst" @('pagina 8', 'pagina 9')
# Pan the pair (two 2:3 pages side by side, same height as a single page),
# then toggle: zoom and pan must both reset.
Send-Wheel $c.X $c.Y 4
$zt = Get-ZoomPercent $win
CheckTrue "zoomed in on the pair before toggling (>150)" ($zt -gt 150) "zoom=$zt%"
Send-Drag $c.X $c.Y 0 100
$st1 = Get-MarkerStats
$vp = Get-Viewport
CheckTrue "pair panned before toggling (marker > 50 px from the viewport centre)" (($st1.Count -gt 0) -and ([math]::Abs($st1.MeanY - $vp.CenterY) -gt 50)) "marker meanY=$(R1 $st1.MeanY) viewport centre=$(R1 $vp.CenterY)"
Send-Keys "m"
Check-ButtonPresent "mode button reads 'Pagina unica'" 'Pagina unica'
Check-Counter "single mode keeps first page of the pair" "8 / 10"
Check-Images "single mode shows only page 8" @('pagina 8')
Check-Zoom "toggling mode resets zoom" 100
Check-PageCentred "toggling back to single resets the pan: page 8 centred"
Send-Keys "{LEFT 7}"; Check-Counter "back to first page" "1 / 10"
Check-Images "first page image" @('pagina 1')

# ---- 6.7 Per-page error placeholder (single mode) --------------------------
Write-Host "`n=== 6.7: per-page error placeholder, single mode ==="
Send-Keys "{RIGHT 6}"
Check-Counter "navigate to the corrupted page" "7 / 10"
Check-ButtonPresent "retry button present on corrupted page" 'Tentar novamente'
$msg = (Get-TextNames $win) | Where-Object { $_ -like '*carregar esta*' } | Select-Object -First 1
CheckTrue "error message text present" ([bool]$msg) "text='$msg'"
Check-Images "no image for the corrupted page" @()
CheckTrue "Anterior still enabled on the error page" (Get-ButtonEnabled $win 'Anterior') "IsEnabled=True"
CheckTrue "Proxima still enabled on the error page" (Get-ButtonEnabled $win 'Proxima') "IsEnabled=True"
# REAL mouse click (not UIA Invoke): Task 4 had a defect where the viewport's
# pointer capture swallowed real clicks on this button, which Invoke cannot
# see. The retry's effect is not observable here (the page really is corrupt,
# so the placeholder comes back); that the click reaches the button's onClick
# was verified by the instrumented real-click run in task-4-report.md, not by
# this script. Here we assert only that the app stays healthy after it.
Click-Button $win 'Tentar novamente'
Start-Sleep -Milliseconds 800
Check-ButtonPresent "still a placeholder after a real-click retry (page is really corrupt)" 'Tentar novamente'
Check-Counter "counter unchanged after retry" "7 / 10"
Check-Images "still no image for the corrupted page after retry" @()
CheckTrue "Anterior still enabled after retry" (Get-ButtonEnabled $win 'Anterior') "IsEnabled=True"
CheckTrue "Proxima still enabled after retry" (Get-ButtonEnabled $win 'Proxima') "IsEnabled=True"
Send-Keys "{RIGHT}"; Check-Counter "navigate past the error page" "8 / 10"
Check-ButtonAbsent "placeholder gone on a good page" 'Tentar novamente'
Check-Images "good page image" @('pagina 8')

Write-Host "`n=== SUMMARY: $global:pass passed, $global:fail failed ==="
if ($global:fail -gt 0) { exit 1 }
exit 0
