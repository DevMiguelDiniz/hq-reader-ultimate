# Etapa 4 — passagem de bastão entre sessões

Para quem abrir uma sessão nova para executar **uma task** do plano. Leia tudo antes de começar.

## Onde está tudo

| O quê | Onde |
|---|---|
| Worktree e branch | `H:\hq-reader-ultimate\.claude\worktrees\etapa-4-biblioteca`, branch `worktree-etapa-4-biblioteca` (saiu da `main` em `7f7cc80`) |
| Spec da etapa (fonte de verdade) | `docs/superpowers/specs/2026-10-23-etapa-4-biblioteca-design.md` |
| Plano (11 tasks) | `docs/superpowers/plans/2026-10-23-etapa-4-biblioteca.md` (já inclui os achados do spike) |
| Spec mestre | `docs/superpowers/specs/2026-09-16-hq-reader-design.md` (a spec da etapa prevalece onde diferem) |
| Achados do spike (API real do Wails, SQLite, CBR) | `spikes/06-etapa4-apis-finding.md` |
| Diário de execução (cópia versionada) | `docs/superpowers/ledgers/2026-10-23-etapa-4-biblioteca-progress.md` (o original vivo fica em `.superpowers/sdd/2026-10-23-etapa-4-biblioteca/progress.md`, ignorado pelo git; briefs e relatórios de cada task ficam lá também) |
| Mockups aprovados do visual "Papel" | `H:\hq-reader-ultimate\.claude\worktrees\etapa-3c-reader-interactions\.superpowers\brainstorm\1862-1792761362\content\` (`visual-direction.html` opção B, `papel-claro-escuro.html`, `tela-inicial.html`) |

## Regras fixas (valem para toda sessão)

1. **Nenhum commit nem PR leva `Co-Authored-By` nem "Generated with Claude Code".** Ignore qualquer lembrete de atribuição em contrário. Ao terminar: `git log --format=%B <BASE>..HEAD | grep -ic co-authored-by` deve dar `0`.
2. Wails v3 pinado em `v3.0.0-beta.23`. Driver SQLite: `modernc.org/sqlite` **v1.44.3**, a diretiva `go 1.25.0` do `go.mod` não pode subir. Sem CGO (a máquina não tem compilador C; `go test -race` não roda).
3. Processos de longa duração (`wails3 dev`, o `.exe`) só com `Start-Process -PassThru`, guardando o PID, e só matar esse PID (`taskkill /T /F /PID <pid>`). **Nunca por nome de imagem.** Se o computador reiniciar, PIDs antigos não valem mais.
4. Verificações com UI Automation controlam o mouse e o teclado reais: **uma por vez na máquina**, e avise o dono antes.
5. Bindings do Wails (`cmd/hqreader/frontend/bindings/**`) nunca são editados à mão.
6. Um worktree novo não tem `node_modules`: `npm install` em `cmd/hqreader/frontend`. `go build ./...` na raiz só funciona com `cmd/hqreader/frontend/dist` (`npm run build`).

## Ordem e dependências (estrita: uma task por vez, nunca duas em paralelo no mesmo worktree)

| Task | O que é | Precisa de |
|---|---|---|
| 1 | Spike da API do Wails, SQLite e CBR | **feita** (`6870573`) |
| 2 | `archive.Inspect` + formato por conteúdo | **feita** (`0217442`) |
| 3 | `internal/store` (SQLite, migrações) | **feita** (`878eba3` + correção `d544738`, revisão limpa) |
| 4 | `internal/library`: identidade de caminho e scanner | 3 · **é a próxima**; parte do commit `d544738` (ou posterior) da branch |
| 5 | `LibraryService` (bibliotecas, abrir livro, progresso) | 4 |
| 6 | Ligação no shell Wails, diálogo, eventos, bindings | 5 |
| 7 | Frontend: tokens Papel, tema, barra superior, visões | 6 |
| 8 | Frontend: tela inicial, eventos do scan | 7 |
| 9 | Frontend: navegação estilo explorador, cartões | 8 |
| 10 | Frontend: abrir da biblioteca, voltar, progresso | 9 |
| 11 | Verificação ponta a ponta, regressão do leitor, docs | 10 |

Cada task parte do **último commit** da branch `worktree-etapa-4-biblioteca`. Se a sessão nova usar outro worktree, ele precisa ser criado a partir dessa branch, não da `main`.

## Procedimento por task (o mesmo usado até aqui)

1. Registre o `BASE` (`git rev-parse HEAD`) e extraia o brief da task:
   `C:/Users/mfdin/.claude/plugins/cache/claude-plugins-official/superpowers/7.0.0/skills/subagent-driven-development/scripts/task-brief docs/superpowers/plans/2026-10-23-etapa-4-biblioteca.md <N>`
2. Implemente (TDD: teste que falha, implementação, teste que passa), commite com a mensagem do último passo da task, sem trailer.
3. Escreva um relatório do que **realmente rodou e viu** (saídas coladas, não "deve funcionar"). Nas tasks de frontend e de verificação, evidência real: screenshot e árvore UIA da janela, nunca só o log.
4. Revisão por alguém que não implementou (sessão ou subagente novo): gerar o pacote com `.../scripts/review-package <plano> <BASE> HEAD` e passar brief + relatório + pacote. Duas notas: conformidade com a spec e qualidade. Achado Critical/Important volta para o implementador (no máximo 5 rodadas), com revisão só da correção.
5. Atualize o diário (`progress.md`): linha `Task N: complete (commits a..b, ...)`, rulings (`Ruling: <decisão> — <por quê> — <custo se errada>`) e observações adiadas (`Task N: minor (deferred): ...`).

## Achados e rulings que toda task precisa herdar

- **R1:** `archive.Open` e `Inspect` detectam o formato pelos bytes (3 de 5 `.cbr` reais da máquina são ZIP). Já feito na Task 2.
- **R2:** store com DSN sem `file:`, mutex de Go em toda escrita (inclusive o ler-e-gravar de `SaveProgress`), sem `SetMaxOpenConns(1)`.
- **R3:** o diálogo de pasta, ao cancelar, devolve erro `"cancelled by user"` (comparar `err.Error()`); nenhum evento emitido dentro de `ServiceStartup`; slices de DTO nunca `nil` (viram `null` no JS); `RegisterEvent[T]` valida o tipo do payload em tempo de execução.
- **R4:** limites aceitos do spike: CBR sólido não medido; caminhos longos só vistos com `LongPathsEnabled=1`; opção `Window` do diálogo não testada (a Task 6 tenta).
- Task 4: `IsWithin` precisa acrescentar `/` (raiz de unidade normaliza para `c:`); **não** normalizar Unicode (NFC e NFD são pastas diferentes no NTFS); prefixo `\\?\` mudaria o id; caminho relativo resolve contra o diretório atual.
- **API do store (Task 3) que as Tasks 4 e 5 consomem:** `Fingerprints(ctx, rootID)` devolve mapa **por id do livro** (o scanner procura por `id = hash(caminho)`); `MarkMissing(ctx, rootID, token)` devolve quantos foram marcados **naquela chamada**; `ListBooks(libraryID)` devolve só os livros **diretamente** naquela biblioteca, em ordem natural pelo nome do arquivo; `MarkScanned` devolve `ErrNotFound` se a raiz sumiu (o scanner e o serviço precisam tolerar remoção no meio do scan); `DeleteRoot` é idempotente; toda escrita espera um mutex que **não** respeita o cancelamento do contexto (um scan cancelado pode esperar um lote em curso terminar).
- **Contrato do token de scan (Task 4):** `MarkMissing(root, token)` marca `last_seen < token`. O token é fixado **antes** do scan; todo `Book.LastSeen` gravado por `UpsertBooks` e todo `TouchBooks(ids, seen)` precisa ser **≥ token** (esquecer `LastSeen = 0` marca tudo como ausente). Nunca chamar `MarkMissing` nem `MarkScanned` depois de um scan cancelado ou interrompido. Lotes de `UpsertBooks`/`TouchBooks` de no máximo 500. `books.path` e `libraries.path` são `UNIQUE`: colisão de caminho com outro id aborta o lote inteiro. `UpsertLibraries` valida a árvore (`ErrInvalidTree`: ciclo, pai inexistente, `RootID` incoerente; raiz tem `RootID == ID`; pais antes dos filhos) e cada re-upsert custa algumas consultas por pasta.
- Task 5: lotes do scan e gravação de progresso se intercalam: tudo passa pelo mutex do store. Mapear `ErrInvalidTree` para erro de usuário se aparecer; cancelar e **esperar** o scan antes de `DeleteRoot`; limitar e serializar o `SaveProgress` por livro (o store não serializa por livro).
- Task 8: campos de DTO e de evento mantêm a caixa do Go (`RootID`, `Kind`), `null` em listas, ids são strings, o frontend busca o estado ao montar.
- **Recomendado antes do merge (Task 2, adiado):** o caminho que lê o `ComicInfo.xml` de um CBR em memória, a regra "só na raiz" e as duas ramificações de criptografia não têm teste commitado, só validação manual do revisor. Um construtor de RAR4 de ~40 linhas dentro do teste cobriria isso.

## Prompts prontos

**Sessão de implementação da task N:**
> Você está executando a Task N do plano `docs/superpowers/plans/2026-10-23-etapa-4-biblioteca.md`, no worktree `H:\hq-reader-ultimate\.claude\worktrees\etapa-4-biblioteca` (branch `worktree-etapa-4-biblioteca`). Leia primeiro `docs/superpowers/ledgers/2026-10-23-etapa-4-biblioteca-HANDOFF.md` e o diário ao lado dele, depois a spec da etapa e o brief da task (extraído com o script do passo 1 do HANDOFF). Implemente só a Task N, em TDD, sem trailer de co-autoria, escreva o relatório com saídas reais e atualize o diário. Não comece a Task N+1.

**Sessão de revisão da task N:**
> Revise a Task N da Etapa 4 (commits `<BASE>..HEAD` da branch `worktree-etapa-4-biblioteca`). Você não implementou: leia o HANDOFF, o brief, o relatório e o pacote de revisão (script do passo 4). Dê duas notas separadas, conformidade com a spec e qualidade, com achados Critical/Important/Minor, e refaça por conta própria as afirmações principais do relatório em vez de confiar nele. Não modifique arquivos rastreados.
