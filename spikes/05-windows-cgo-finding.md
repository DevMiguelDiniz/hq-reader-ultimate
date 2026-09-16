# Spike 5 — Exigência de CGO no build Windows

## Test Results

**Build com CGO_ENABLED=0:** sucesso

The build completed successfully. Output shows:
- Frontend build successful (`✓ built in 178ms`)
- Native Go binary compiled successfully (`go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o "bin/wails-poc.exe"`)
- Binary created: `bin/wails-poc.exe` (11,074,560 bytes)

**Build com CGO_ENABLED=1:** sucesso

The build completed successfully with identical output. Output shows:
- Frontend build successful (`✓ built in 87ms`)
- Native Go binary compiled successfully
- Binary created: `bin/wails-poc.exe` (11,074,560 bytes)

## Conclusão

**O Wails v3 no Windows NÃO exige CGO.**

Both builds completed without errors, indicating that Wails v3.0.0-beta.23's own internals do not require CGO to be enabled on Windows. The build toolchain functions perfectly well with pure Go binaries.

## Impacto na escolha do driver SQLite (`mattn/go-sqlite3`, que exige CGO)

**Decisão**: Escolher `mattn/go-sqlite3` INTRODUZIRIA uma exigência NEW de um compilador C/toolchain (como GCC via MSYS2 ou TDM-GCC) no Windows, que não existiria de outra forma.

- **Situação atual**: Desenvolvedores e usuários finais podem fazer build no Windows com apenas o Go instalado.
- **Com `mattn/go-sqlite3`**: Um compilador C se tornaria obrigatório no Windows, impactando:
  - Overhead de setup para desenvolvimento
  - Possibilidade de erros em build para usuários finais
  - Complexidade aumentada da toolchain

**Recomendação** (para §4.1 do design): Considerar drivers SQLite puros em Go como alternativa (ex: `crawshaw/sqlite`, `glebarez/sqlite`, ou `mattn/go-sqlite3` com `sqlite3_omit_load_extension` quando apropriado) se o objetivo é evitar uma dependência de toolchain C no Windows. Se `mattn/go-sqlite3` for escolhido apesar dessa descoberta, documentar explicitamente a exigência de um compilador C para builds no Windows.
