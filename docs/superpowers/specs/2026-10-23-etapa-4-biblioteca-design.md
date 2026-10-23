# Etapa 4 — Biblioteca e scanner (design)

Data: 2026-10-23
Status: rascunho para revisão
Spec mestre: [2026-09-16-hq-reader-design.md](2026-09-16-hq-reader-design.md) (este documento detalha a Etapa 4 do §6 e prevalece sobre ele onde houver diferença; as diferenças estão listadas em §9).

## 1. Intenção

O hqreader hoje abre um único quadrinho passado por variável de ambiente (`HQREADER_BOOK`). Esta etapa transforma o app em uma **biblioteca de verdade**: o usuário aponta pastas do computador, o app as escaneia aos poucos, mostra os quadrinhos encontrados, e ele abre e lê qualquer um, voltando sempre na página em que parou.

O que motivou o projeto, nas palavras do dono: o **visual**. A maioria dos leitores de quadrinhos tem aparência "crua"; este deve ser moderno, atraente e intuitivo. Por isso o design visual (§6) é requisito desta etapa, não acabamento posterior.

Sucesso, do ponto de vista de quem usa:
- Adiciono minhas pastas de HQs uma vez, e a biblioteca vai se enchendo na minha frente, sem eu ficar esperando.
- Navego como num explorador de arquivos e acho o que quero.
- Abro um quadrinho e continuo exatamente de onde parei, mesmo dias depois.
- O app é bonito de olhar, nos temas claro e escuro.

## 2. Decisões já tomadas pelo dono

| Tema | Decisão |
|---|---|
| Driver SQLite | **`modernc.org/sqlite`** (Go puro, sem CGO), via `database/sql`. Substitui `mattn/go-sqlite3` do spec mestre (§2, §4). Motivo: esta máquina não tem compilador C, e Go puro simplifica o empacotamento da Etapa 8. |
| Várias pastas | O usuário adiciona **várias pastas**; cada uma é uma biblioteca (raiz). |
| Sub-bibliotecas | As pastas dentro de uma biblioteca continuam pertencendo a ela, mas aparecem como **sub-bibliotecas** (seguindo a estrutura de pastas do disco). |
| Navegação | **Estilo explorador**: dentro de uma biblioteca vejo as sub-bibliotecas como pastas e os quadrinhos ao lado; entro numa sub-biblioteca e volto por um caminho de navegação (breadcrumb). |
| Carregamento | O scan é **gradual**: os quadrinhos aparecem conforme são encontrados, com um indicador de carregamento; os já carregados ficam clicáveis e abrem para leitura **durante** o scan. |
| Informação no cartão | Capa, nome e informações do quadrinho. |
| Capas | **Placeholder** nesta etapa. Capas reais e miniaturas ficam na Etapa 6 (como no roteiro). |
| Progresso | Ao reabrir, o leitor volta **exatamente na página** onde o usuário parou. |
| Atualização | **Rescan manual**, com botão "Atualizar". Sem observação automática de pastas (já era decisão do spec mestre, §4). |
| Visual | Direção **"Papel"**: aparência de HQ impressa (serifa, bordas grossas com sombra chapada, acento vermelho-alaranjado), em **tema claro e tema escuro** (o escuro é "papel de noite": marrom-tinta quente, não preto frio). Os mockups aprovados estão em `.superpowers/brainstorm/*/content/papel-claro-escuro.html` e `visual-direction.html` (opção B). |

## 3. Objetivos, não-objetivos e anti-objetivos

**Objetivos**
1. Gerenciar bibliotecas (adicionar pasta, remover biblioteca) e persisti-las.
2. Scanner gradual, cancelável, que se comunica com a interface em tempo real.
3. Tela de biblioteca (explorador) e troca fluida entre biblioteca e leitor.
4. Progresso de leitura persistido e retomada na página exata.
5. Sistema visual "Papel" claro/escuro aplicado ao app inteiro.
6. Corrigir a pendência da Etapa 3c: o estado por livro do frontend (`pageAspects`, `failed`, `retries`) passa a ser indexado por livro, já que agora há troca de livro.

**Não-objetivos (ficam fora desta etapa)**
- Capas reais, miniaturas, cache (Etapa 6).
- Coleções criadas pelo usuário (Etapa 5). Atenção: bibliotecas/sub-bibliotecas são derivadas das pastas do disco; coleções são agrupamentos manuais, conceitos separados.
- Observação automática de pastas (`fsnotify`), detecção de arquivo movido.
- Busca, filtros e ordenação configurável (a ordem é fixa: ver §5.4). *Chamada minha.*
- Guardar por livro o modo de página (única/dupla). A Etapa 3c disse que isso ficaria "para a Etapa 4/5"; decidi **adiar para a Etapa 5**, junto de `book_settings`, para esta etapa não crescer. Até lá o modo volta a "única" a cada abertura. *Chamada minha.*
- Janela destacável (Etapa 7), instalador (Etapa 8), modo contínuo/mangá.
- Validação em Linux/WebKitGTK.

**Anti-objetivos (seria fracasso mesmo funcionando)**
- Aparência "crua"/genérica: shadcn cinza padrão sem identidade.
- Travar a interface durante o scan, ou obrigar a esperar o fim dele para abrir algo.
- Perder o ponto de leitura, ou voltar sempre à página 1.
- Um scan que para inteiro por causa de um arquivo ruim.

## 4. Restrições

- Go, Wails v3 pinado em `v3.0.0-beta.23`; frontend React 19 + TypeScript + Tailwind v4 + shadcn/ui + Zustand + Motion. Máquina de desenvolvimento: Windows, sem compilador C (`CGO_ENABLED=0`); por isso o código novo não pode depender de CGO.
- Serviços de backend independentes de janela (spec mestre §3.2).
- Identificadores: `id` do livro = hash do **caminho** (spec §3.5); o `id` da biblioteca idem. O `bookID` de **sessão** do leitor (opaco, `reader.Service`) continua existindo e distinto do `id` persistente do livro.
- SQLite em modo WAL, um único `*sql.DB`.
- Verificação do frontend por script de UI Automation real com saída bruta commitada, sem framework de teste de frontend (política do projeto). No backend, testes Go table-driven são desejados para scanner, store e migrações.
- Os commits não levam `Co-Authored-By` nem "Generated with Claude Code" (regra do dono).

## 5. Comportamento

### 5.0 Tela inicial (aprovada pelo dono, mockup `tela-inicial.html`)

- O nível mais alto da navegação é **"Minhas HQs"**: uma tela que lista as **bibliotecas raiz** como cartões grandes (nome, caminho no disco, "N HQs · M sub-bibliotecas", estado do scan, menu "⋯"). Entrar num cartão leva à navegação estilo explorador daquela biblioteca; o caminho de navegação (breadcrumb) começa em "Minhas HQs".
- No topo, em todas as telas do app: o logo "hqreader", o seletor de tema (Claro / Escuro / Auto) e, na tela inicial, **"+ Adicionar biblioteca"** e **"↻ Atualizar tudo"**. Dentro de uma biblioteca, "↻ Atualizar" reescaneia a biblioteca raiz em que o usuário está.
- O menu "⋯" do cartão de uma biblioteca raiz oferece **Remover biblioteca** (com confirmação; nunca apaga arquivos do disco).
- **Primeira execução (nenhuma biblioteca):** estado vazio com a ilustração de pilha de capas, o texto "Sua estante está vazia" com uma frase explicando que basta apontar uma pasta de CBZ/CBR, e o botão "+ Adicionar biblioteca".
- Cartão em scan mostra "N encontrados…" e uma barra fina; cartão com scan interrompido mostra o aviso "scan incompleto" (§5.2).

### 5.1 Bibliotecas e sub-bibliotecas

- "Adicionar biblioteca" abre o seletor nativo de pastas. A pasta escolhida vira uma **biblioteca raiz** e é escaneada.
- Durante o scan, cada pasta que contenha ao menos um `.cbz`/`.cbr` (diretamente ou em descendentes) vira uma **sub-biblioteca**, com pai = a pasta imediatamente acima. Pastas sem nenhum quadrinho em toda a subárvore não aparecem. *Chamada minha.*
- Remover uma biblioteca raiz apaga do banco a biblioteca, suas sub-bibliotecas, seus livros e o progresso de leitura deles. Nunca toca nos arquivos do disco.
- Adicionar uma pasta que já está dentro de uma biblioteca existente, ou que contém uma biblioteca existente, é rejeitado com mensagem clara (evita duplicar livros). *Chamada minha.*

### 5.2 Scanner

- Percorre a raiz recursivamente. Considera arquivos `.cbz` e `.cbr` (extensão sem diferenciar maiúsculas).
- Para cada arquivo: se já existe no banco com mesmo `size` e `mtime`, só confirma que continua presente (sem reabrir). Caso contrário, faz **validação rasa**: lista as páginas e lê o `ComicInfo.xml` quando existir, sem decodificar imagens, e grava (`upsert` por `id` = hash do caminho).
- **Inspeção leve de arquivos.** Hoje `archive.Open` num CBR **extrai o arquivo inteiro** para uma pasta temporária já na abertura (o spec mestre §3.3 descreve extração só na primeira leitura de página, mas o código atual extrai no `Open`). Usá-lo no scan seria lento e pesado em disco. Esta etapa inclui uma função de inspeção leve em `internal/archive` (por exemplo `Inspect(path)`, devolvendo contagem de páginas e `ComicInfo`) que, para CBR, lê só os cabeçalhos e extrai apenas o `ComicInfo.xml`, sem gravar as imagens; para CBZ usa o índice do zip. O scanner usa a inspeção; `Open` continua só para a leitura.
- **Duas fases.** Primeiro uma caminhada rápida só pela estrutura de pastas e nomes de arquivo (sem abrir nada), que descobre as sub-bibliotecas e **quantos arquivos** existem; depois, a validação com paralelismo limitado. Assim o indicador mostra progresso real ("validados / encontrados"). Os cartões esqueleto ("Carregando…") são só visuais: mostram os próximos arquivos ainda não validados, não são linhas do banco. Um livro só vira cartão de verdade quando validado e gravado. *Chamada minha.*
- **Scan interrompido** (app fechado ou cancelado): nunca marca livros como `missing`. A biblioteca guarda se o último scan terminou (`scanned_at` só é preenchido ao concluir) e, se não terminou, mostra um aviso discreto "scan incompleto — Atualizar". **Nada escaneia sozinho** ao abrir o app; só o botão. *Chamada minha.*
- Falha na validação rasa → o livro é gravado com `broken=1` e o scan continua (spec §3.6).
- No fim do scan, livros da raiz que não foram vistos recebem `missing=1`; os vistos voltam a `missing=0`. Livros `missing` continuam listados, esmaecidos e não abrem; sub-bibliotecas só com livros ausentes continuam listadas. *Chamada minha:* não há ação de "remover da lista" por livro nesta etapa.
- É **gradual**: cada livro gravado é anunciado ao frontend assim que fica pronto (em lotes pequenos, por exemplo a cada ~50 ms), e o progresso do scan (quantos encontrados; fase) também. O scan roda em segundo plano, com paralelismo limitado para a leitura dos arquivos, e **não bloqueia** nenhuma outra operação do app, inclusive abrir e ler livros.
- **Cancelável** (`context`) e nunca concorrente consigo mesmo na mesma raiz: pedir "Atualizar" enquanto a raiz está sendo escaneada é ignorado ou reaproveita o scan em curso. Fechar o app durante o scan o interrompe sem corromper o banco.
- Um arquivo ou pasta ilegível (permissão, caminho longo) é registrado e pulado; não interrompe o scan.

### 5.3 Abrir e ler

- Clicar num quadrinho (não `missing`, não `broken`) abre o leitor existente (Etapa 3c) com aquele livro. O leitor ganha um botão de voltar para a biblioteca, que fecha o livro e devolve o usuário ao ponto da navegação onde estava.
- `HQREADER_BOOK` deixa de ser o caminho principal. *Chamada minha:* mantê-lo apenas como atalho de desenvolvimento/verificação, que abre direto no leitor, é aceitável; o plano decide.
- Livros `broken` mostram o motivo (senha, corrompido, formato não suportado) de forma legível, usando os erros tipados já existentes (`ErrPasswordProtected`, `ErrCorrupted`, `ErrUnsupportedFormat`).

### 5.4 Cartão do quadrinho e ordem

- Cartão: placeholder de capa (gradiente determinístico a partir do título/arquivo, com iniciais, como nos mockups), título (do `ComicInfo.xml` quando houver, senão o nome do arquivo sem extensão), e o progresso ("34 / 120", "Lido", "Novo"), com barra fina. Opcionalmente série/número quando houver `ComicInfo`.
- Estados visuais: carregando (esqueleto, enquanto o scan ainda lista), `missing` (esmaecido), `broken` (aviso).
- Ordem fixa: sub-bibliotecas primeiro (ordem natural pelo nome), depois quadrinhos (ordem natural pelo nome do arquivo).

### 5.5 Progresso de leitura

- A página atual de cada livro é persistida (`reading_progress`) conforme o usuário vira páginas, **com debounce** (spec §3.5: escrita frequente) e sempre gravada ao fechar o livro e ao encerrar o app.
- Ao abrir um livro com progresso, o leitor começa naquela página. Isso fecha o gap documentado no spec §3.2 (`CurrentPage` nunca lido): o backend passa a ser a fonte da página inicial. No modo duplo, o progresso guarda o primeiro índice da visão.
- Livro sem progresso começa na página 0.
- **"Lido"** significa que o usuário **já chegou à última página**, e permanece assim (não some se ele reler). Para isso `reading_progress` guarda, além de `current_page`, a **página mais distante já vista** (`furthest_page`); "Lido" = `furthest_page` ≥ última página. No modo duplo o leitor reporta o **último índice da visão** como página vista, para o par final contar. Um livro "Lido" reabre na página salva (onde o usuário parou), não à força na primeira. O cartão mostra "34 / 120" a partir de `current_page`. *Chamada minha.*

## 6. Design visual

Direção **Papel**, aprovada pelo dono em duas versões.

- **Tokens** (cores, raios, sombras) definidos como variáveis CSS para claro e escuro, alimentando também as variáveis do shadcn/ui, de modo que todos os componentes herdem o visual. Claro: fundo cor de papel (`#f3ede2`), tinta (`#1d1a16`), acento `#d9432b`/`#f0a63c`. Escuro: fundo marrom-tinta (`#17130f`), texto `#f3ece0`, acento `#ff6a45`/`#f0b04a`, sombra dos quadrinhos no acento. Valores exatos de partida nos mockups; o plano pode refiná-los.
- **Tipografia:** serifa (Georgia/sistema, sem fonte web nova) para títulos e identidade; sans do sistema para rótulos pequenos. A fonte web (Geist) que a CLI do shadcn instalou na Etapa 3b deixa de ser usada se não for necessária. *Chamada minha.*
- **Formas:** bordas de 2 px, raio quase nulo, sombra chapada deslocada (3–4 px) nos cartões e capas, rótulos de seção em caixa alta com acento. Estados de foco, hover e disabled com o mesmo vocabulário.
- **Tema:** seletor com três opções (Claro, Escuro, Sistema), padrão **Sistema**. A preferência é guardada no `localStorage` do frontend. *Chamada minha (palpite do padrão).*
- **Escopo visual:** biblioteca, barra de scan, diálogos e o leitor (a linha de controles e o fundo) adotam o mesmo sistema. A área da página do leitor continua escura/neutra para não competir com a imagem. *Chamada minha.*
- Animações (Motion) onde fizer sentido e discretas: entrada dos cartões conforme chegam do scan, transição biblioteca ↔ leitor. Respeitar `prefers-reduced-motion`.

## 7. Arquitetura (em alto nível)

- **Backend (Go), independente de janela:**
  - `internal/store`: abre o SQLite (`modernc`), aplica migrações versionadas (`PRAGMA user_version`), modo WAL, um `*sql.DB`. O arquivo fica em `os.UserConfigDir()/hqreader/library.db`. *Chamada minha.*
  - `internal/library`: `LibraryService` (adicionar/remover/listar bibliotecas, listar conteúdo de uma biblioteca ou sub-biblioteca, iniciar/cancelar scan, ler/gravar progresso) e o scanner. Depende de `internal/archive` e de uma interface de store, para ser testável sem Wails.
  - `internal/reader`: continua o serviço de sessão de leitura. Precisa de um gancho para avisar a camada de biblioteca quando a página atual muda (para persistir o progresso), sem passar a depender dela, e de uma forma de abrir um livro a partir da biblioteca e receber de volta a página inicial.
  - Esquema novo ou alterado (aproximado, o plano fecha): `libraries(id, path UNIQUE, name, parent_id NULL, root_id, added_at, scanned_at)`; `books` (spec §3.5) ganha `library_id`; `reading_progress` conforme §3.5. Chaves estrangeiras com `ON DELETE CASCADE`.
  - O shell Wails (`cmd/hqreader`) registra os serviços, fornece o diálogo de pasta e a emissão de eventos para o frontend.
- **Frontend:**
  - Uma visão de topo alterna entre **Biblioteca** e **Leitor**; a navegação dentro da biblioteca (caminho, biblioteca/sub-biblioteca atual) vive numa store Zustand própria.
  - Os eventos do scan alimentam a store e fazem os cartões entrarem gradualmente.
  - O estado por livro do leitor é separado por livro (`bookID`) e descartado ao trocar de livro.
  - Os bindings TypeScript do Wails são regenerados (nunca editados à mão).

## 8. Erros

- Erros do scanner são locais ao arquivo/pasta (registrados, o livro vira `broken`/é pulado) e nunca derrubam o scan. Erros globais (banco inacessível, pasta raiz inexistente na hora do scan) são mostrados ao usuário de forma visível e não destrutiva (aviso dispensável, no mesmo espírito do `navError` da Etapa 3c).
- Falha ao persistir progresso não interrompe a leitura.
- Banco ausente na primeira execução é criado; esquema mais novo que o app (banco de versão futura) bloqueia o uso com mensagem clara, sem alterar o arquivo.

## 9. Diferenças em relação ao spec mestre

1. Driver SQLite: `modernc.org/sqlite` em vez de `mattn/go-sqlite3` (§2, §4 e a nota de risco do §4 ficam resolvidos: o spec mestre deve ser atualizado).
2. Modelo de bibliotecas com hierarquia (raízes + sub-bibliotecas) e tabela `libraries`, inexistente no §3.5; `books` ganha `library_id`.
3. O "grid plano" do §6 vira navegação estilo explorador, por pedido do dono.
4. Capas: placeholder aqui; reais na Etapa 6 (inalterado no roteiro).
5. Progresso de leitura entra nesta etapa (o §6 não o listava na Etapa 4; a tabela `reading_progress` já estava no §3.5).

## 10. Verificação

- **Backend:** testes Go table-driven para o scanner (contra diretórios temporários com CBZ sintéticos, incluindo um corrompido e um protegido por senha quando viável), para o store/migrações (SQLite em arquivo temporário), para `LibraryService` (adicionar/remover bibliotecas com a regra de sobreposição, hierarquia de sub-bibliotecas, `missing`/`broken` no rescan, cancelamento) e para o progresso (debounce e retomada).
- **Frontend e fim a fim:** script PowerShell de UI Automation real, em `wails3 dev` e em `wails3 build`, com saída bruta commitada: adicionar biblioteca, ver os cartões chegando durante o scan, abrir um livro durante o scan, navegar em sub-biblioteca e voltar, ler algumas páginas, voltar à biblioteca, reabrir o app e conferir que o livro abre na página salva, atualizar após adicionar/remover arquivo. Verificação visual por captura de tela (PrintWindow) nos dois temas.
- O diálogo nativo de pasta é difícil de automatizar: o plano deve prever um jeito de apontar a pasta na verificação (por exemplo, um parâmetro de desenvolvimento) e dizer que o diálogo em si foi verificado manualmente.

## 11. Riscos e perguntas para o plano

- **API do Wails v3 `beta.23`:** confirmar como se abre o diálogo nativo de pasta, como um serviço emite eventos para o frontend e como o frontend os assina. Recomenda-se um spike curto no início do plano (padrão da Etapa 0), pois a API em beta pode diferir da documentação.
- **Desempenho do `modernc`:** medir a gravação em lote de milhares de livros; se for um problema, agrupar em transações.
- **Custo da inspeção de CBR no scan:** mesmo a inspeção leve (§5.2) precisa decodificar entradas anteriores para chegar ao `ComicInfo.xml` num CBR **sólido**. Medir com amostras reais; se necessário, limitar paralelismo ou adiar a leitura de metadados para depois da listagem. A divergência entre o spec mestre (extração preguiçosa) e o código (extração no `Open`) deve ser corrigida no spec mestre.
- **Caminhos no Windows:** maiúsculas/minúsculas, barras, caminhos longos e unidades de rede ao calcular o hash do caminho; normalizar antes de hashear.
- **Integração com a Etapa 3c:** esta etapa depende do código do leitor da Etapa 3c, ainda na PR #1.
