# Spike 5 — Exigência de CGO no build Windows

## Critical Finding: No C Compiler Detected on Build Machine

**Machine state (verified):**
```
gcc: NOT FOUND
cc: NOT FOUND
clang: NOT FOUND
go env CC: gcc (default, not found in PATH)
go env CGO_ENABLED: 0 (default when no C compiler detected)
```

**Implication**: The CGO_ENABLED=1 build below does NOT exercise real cgo compilation, because:
1. No C compiler exists on this Windows machine
2. `spikes/wails-poc` contains no `import "C"` and no cgo-dependent packages
3. Setting CGO_ENABLED=1 without an actual C toolchain present is a no-op

Therefore, the CGO_ENABLED=1 run proves only that **Wails' own framework does not require cgo**, not that a functional C toolchain exists on this machine.

---

## Test Results

### Build 1: CGO_ENABLED=0

**Command:**
```powershell
cd spikes/wails-poc
$env:CGO_ENABLED = "0"
wails3 build
```

**Raw Terminal Output:**
```
Wails v3.0.0-beta.23 › Build
wails3 : "uname": executable file not found in $PATH
[warnings about missing Unix tools suppressed — non-critical on Windows]
"tail": executable file not found in $PATH
task: [windows:common:go:mod:tidy] go mod tidy
task: Task "windows:common:generate:icons" is up to date
task: Task "windows:common:generate:bindings" is up to date
task: Task "windows:common:install:frontend:deps:npm" is up to date
task: Task "build:frontend (DEV= RUNNER=npm)" is up to date
task: [windows:generate:syso] wails3 generate syso -arch amd64 -icon windows/icon.ico -manifest windows/wails.exe.manifest -info windows/info.json -out ../wails_windows_amd64.syso
task: [windows:build:native] go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o "bin/wails-poc.exe"
task: [windows:build:native] powershell Remove-item *.syso

Need documentation? Run: wails3 docs
 ♥   If Wails is useful to you or your company, please consider sponsoring the project: https://github.com/sponsors/leaanthony
```

**Result:** BUILD SUCCEEDED ✓
- Binary created: `bin/wails-poc.exe`
- No C compiler needed or invoked

### Build 2: CGO_ENABLED=1

**Command:**
```powershell
cd spikes/wails-poc
$env:CGO_ENABLED = "1"
wails3 build
```

**Raw Terminal Output:**
```
Wails v3.0.0-beta.23 › Build
wails3 : "uname": executable file not found in $PATH
[warnings about missing Unix tools suppressed — non-critical on Windows]
"tail": executable file not found in $PATH
task: [windows:common:go:mod:tidy] go mod tidy
task: Task "windows:common:generate:icons" is up to date
task: Task "windows:common:generate:bindings" is up to date
task: Task "windows:common:install:frontend:deps:npm" is up to date
task: Task "build:frontend (DEV= RUNNER=npm)" is up to date
task: [windows:generate:syso] wails3 generate syso -arch amd64 -icon windows/icon.ico -manifest windows/wails.exe.manifest -info windows/info.json -out ../wails_windows_amd64.syso
task: [windows:build:native] go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o "bin/wails-poc.exe"
task: [windows:build:native] powershell Remove-item *.syso

Need documentation? Run: wails3 docs
 ♥   If Wails is useful to you or your company, please consider sponsoring the project: https://github.com/sponsors/leaanthony
```

**Result:** BUILD SUCCEEDED ✓
- Binary created: `bin/wails-poc.exe`
- **No C compiler invoked** (no cgo-dependent code in this app, no C toolchain available on machine)

---

## Conclusão

### Sobre Wails v3 e CGO

**Wails v3 no Windows NÃO exige CGO para sua própria compilação.**

Both builds completed successfully without invoking any C compiler, because:
- Wails v3's Windows webview binding uses COM and syscall (pure Go), not cgo
- Setting `CGO_ENABLED=0` does not prevent Wails from building

### Sobre mattn/go-sqlite3 — Impacto Real e Imediato

**CRITICAL**: Escolher `mattn/go-sqlite3` INTRODUCES uma exigência imediata de um compilador C no Windows.

**Situação atual (sem mattn/go-sqlite3):**
- Desenvolvedores podem fazer build no Windows com APENAS Go instalado
- Máquinas CI podem usar imagens padrão sem MSYS2/TDM-GCC
- Usuários finais não precisam de um toolchain C

**Com `mattn/go-sqlite3`:**
- **Build falha imediatamente** em máquinas Windows sem C compiler (ex: esta máquina de teste)
- Exige instalação de MSYS2 (recomendado) ou TDM-GCC antes de qualquer build
- Aumenta complexidade de setup para desenvolvedores
- CI/CD pipelines precisam de custom toolchain setup
- Possível impacto em distribuição (cross-compilation para Windows é mais difícil)

**Recomendação** (para §4.1 do design):
- ⚠️ **Se mattn/go-sqlite3 for escolhido**: documentar explicitamente que Windows dev/build requer MSYS2 (gcc, make, etc) — e considerar quão viável é isso para o projeto
- ✓ **Alternativa preferida**: drivers SQLite puros em Go (`crawshaw/sqlite`, `glebarez/sqlite`, etc) que removem essa exigência de toolchain C inteiramente no Windows
- Se mattn/go-sqlite3 for mantido: considerar build tags ou condicional para tests/CI que não exigem SQLite no Windows
