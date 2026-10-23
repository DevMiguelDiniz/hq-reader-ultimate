# spike 06 — APIs da Etapa 4 (descartável)

Módulo Go próprio (fora do módulo raiz), copiado de `spikes/wails-poc` só com o
scaffold de build do Windows. Resultado em `../06-etapa4-apis-finding.md`.

- `main.go`, `service.go`, `frontend/`: app Wails v3 beta.23 com `SpikeService`
  (`PickFolder`, `EmitBurst`, `Sample`, `Slow`, `Report`, `ServiceStartup`/`ServiceShutdown`).
  Toda evidência vai para o log em `SPIKE_LOG` (o frontend reporta via `Report`).
- `verify-ui.ps1`: dirige a janela real (`-Cancel`: clique real + `{ESC}`; `-Select`: OK do diálogo).
- `cmd/dbbench`: benchmark do `modernc.org/sqlite` (`go run ./cmd/dbbench -dir <pasta fora do repo>`).
- `cmd/cbrbench`: varredura de cabeçalhos com `rardecode` vs `archive.Open` (`go run ./cmd/cbrbench <arquivos>`).
- `pathnorm/`: a Decisão 9 (`NormalizePath`/`PathID`) e o teste no Windows (`go test ./pathnorm -v`).

`go.mod` usa o caminho `github.com/DevMiguelDiniz/hq-reader-ultimate/spikes/06-etapa4-apis`
com `replace` para `../..`, o que permite importar `internal/archive` do módulo raiz.
