// SPIKE (Etapa 4, Task 1) — benchmark descartavel do modernc.org/sqlite.
// Uso: go run ./cmd/dbbench -dir <pasta fora do repo>
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS libraries(
  id TEXT PRIMARY KEY, path TEXT NOT NULL, name TEXT NOT NULL,
  parent_id TEXT REFERENCES libraries(id) ON DELETE CASCADE,
  root_id TEXT NOT NULL, added_at INTEGER NOT NULL, scanned_at INTEGER);
CREATE TABLE IF NOT EXISTS books(
  id TEXT PRIMARY KEY,
  library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  path TEXT NOT NULL, format TEXT NOT NULL, title TEXT NOT NULL, series TEXT NOT NULL, number TEXT NOT NULL,
  size INTEGER NOT NULL, mtime INTEGER NOT NULL, page_count INTEGER NOT NULL,
  added_at INTEGER NOT NULL, last_seen INTEGER NOT NULL,
  missing INTEGER NOT NULL DEFAULT 0, broken INTEGER NOT NULL DEFAULT 0, broken_reason TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS books_library ON books(library_id);
CREATE TABLE IF NOT EXISTS reading_progress(
  book_id TEXT PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
  current_page INTEGER NOT NULL, furthest_page INTEGER NOT NULL, updated_at INTEGER NOT NULL);
`

const upsertBook = `INSERT INTO books(id, library_id, path, format, title, series, number, size, mtime, page_count, added_at, last_seen)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET path=excluded.path, format=excluded.format, title=excluded.title, series=excluded.series,
 number=excluded.number, size=excluded.size, mtime=excluded.mtime, page_count=excluded.page_count,
 last_seen=excluded.last_seen, missing=0`

func dsn(path, extra string) string {
	return "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)" + extra
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func openFresh(dir, name, d string) (*sql.DB, string) {
	path := filepath.Join(dir, name)
	for _, suf := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suf)
	}
	if d == "" {
		d = dsn(path, "")
	}
	db, err := sql.Open("sqlite", strings.ReplaceAll(d, "{path}", path))
	must(err)
	_, err = db.Exec(schema)
	must(err)
	_, err = db.Exec(`INSERT INTO libraries(id,path,name,root_id,added_at) VALUES('L','C:/Comics','Comics','L',1)`)
	must(err)
	return db, path
}

// useWriteMu: serializa toda transacao de escrita num sync.Mutex do Go.
var (
	useWriteMu bool
	writeMu    sync.Mutex
)

func insertBatches(db *sql.DB, start, total, batch int, seen int64) {
	for i := start; i < start+total; i += batch {
		if useWriteMu {
			writeMu.Lock()
		}
		tx, err := db.Begin()
		must(err)
		st, err := tx.Prepare(upsertBook)
		must(err)
		for j := i; j < i+batch && j < start+total; j++ {
			_, err := st.Exec(fmt.Sprintf("%032x", j), "L", fmt.Sprintf(`C:\Comics\Série %d\HQ %05d.cbz`, j%50, j),
				"cbz", fmt.Sprintf("HQ %d", j), "Série", fmt.Sprint(j), int64(1_000_000+j), int64(1_700_000_000+j), 24+j%100, seen, seen)
			must(err)
		}
		st.Close()
		must(tx.Commit())
		if useWriteMu {
			writeMu.Unlock()
		}
	}
}

// ---- 1. DSN: pragmas valem em CADA conexao do pool?
func checkDSN(dir string) {
	fmt.Println("== 1. DSN pragmas per pooled connection")
	sub := filepath.Join(dir, "Pasta Ação com espaço #1")
	must(os.MkdirAll(sub, 0o755))
	sub2 := filepath.Join(dir, "a%20b & c")
	must(os.MkdirAll(sub2, 0o755))
	for _, c := range []struct{ label, d string }{
		{"file:+_pragma (accents/space/#)", dsn(filepath.Join(sub, "dsn.db"), "")},
		{"plain path+_pragma (accents/space/#)", filepath.Join(sub, "dsn2.db") + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"},
		{"plain path+_pragma (%20 & in name)", filepath.Join(sub2, "dsn3.db") + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"},
	} {
		db, err := sql.Open("sqlite", c.d)
		must(err)
		db.SetMaxOpenConns(3)
		ctx := context.Background()
		var conns []*sql.Conn
		for i := 0; i < 3; i++ {
			cn, err := db.Conn(ctx)
			if err != nil {
				fmt.Printf("  %s: conn %d error: %v\n", c.label, i, err)
				break
			}
			conns = append(conns, cn)
		}
		for i, cn := range conns {
			var jm string
			var fk, bt int
			e1 := cn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&jm)
			e2 := cn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
			e3 := cn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&bt)
			fmt.Printf("  %s: conn#%d journal_mode=%s foreign_keys=%d busy_timeout=%d errs=%v/%v/%v\n", c.label, i, jm, fk, bt, e1, e2, e3)
		}
		for _, cn := range conns {
			cn.Close()
		}
		_, err = db.Exec(schema)
		if err == nil {
			_, err = db.Exec(`INSERT INTO books(id,library_id,path,format,title,series,number,size,mtime,page_count,added_at,last_seen) VALUES('b','NOPE','p','cbz','t','','',1,1,1,1,1)`)
			fmt.Printf("  %s: insert book with unknown library_id -> err=%v\n", c.label, err)
		} else {
			fmt.Printf("  %s: schema error: %v\n", c.label, err)
		}
		db.Close()
	}
	entries, _ := os.ReadDir(sub)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	fmt.Printf("  files created in %q: %v\n", sub, names)
	entries, _ = os.ReadDir(sub2)
	names = nil
	for _, e := range entries {
		names = append(names, e.Name())
	}
	fmt.Printf("  files created in %q: %v\n", sub2, names)
	entries, _ = os.ReadDir(dir)
	names = nil
	for _, e := range entries {
		names = append(names, e.Name())
	}
	fmt.Printf("  entries in -dir itself: %q\n", names)
}

// ---- 2. lotes
func benchBatches(dir string) {
	fmt.Println("== 2. insert 5000 books (upsert, prepared stmt per tx)")
	for _, batch := range []int{100, 500, 1000} {
		var times []string
		for run := 0; run < 3; run++ {
			db, _ := openFresh(dir, "batch.db", "")
			t0 := time.Now()
			insertBatches(db, 0, 5000, batch, 1)
			el := time.Since(t0)
			var n int
			must(db.QueryRow("SELECT count(*) FROM books").Scan(&n))
			db.Close()
			times = append(times, fmt.Sprintf("%s(n=%d)", el.Round(time.Millisecond), n))
		}
		fmt.Printf("  batch=%4d: %v\n", batch, times)
	}
	// reupsert (scan repetido: mesmo id, atualiza)
	db, _ := openFresh(dir, "batch.db", "")
	insertBatches(db, 0, 5000, 500, 1)
	t0 := time.Now()
	insertBatches(db, 0, 5000, 500, 2)
	fmt.Printf("  re-upsert of the same 5000 ids (batch=500): %s\n", time.Since(t0).Round(time.Millisecond))
	db.Close()
}

type stats struct {
	queries, locked, busy, other int64
	lat                          []time.Duration
	firstErr                     string
	mu                           sync.Mutex
}

func classify(s *stats, err error) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "locked"):
		atomic.AddInt64(&s.locked, 1)
	case strings.Contains(msg, "busy"):
		atomic.AddInt64(&s.busy, 1)
	default:
		atomic.AddInt64(&s.other, 1)
	}
	s.mu.Lock()
	if s.firstErr == "" {
		s.firstErr = err.Error()
	}
	s.mu.Unlock()
}

func pct(l []time.Duration, p float64) time.Duration {
	if len(l) == 0 {
		return 0
	}
	sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })
	return l[int(float64(len(l)-1)*p)]
}

// ---- 3. 4 leitores + 1 escritor
func benchConcurrent(dir, label, d string, maxOpen int) {
	db, _ := openFresh(dir, "conc.db", d)
	if maxOpen > 0 {
		db.SetMaxOpenConns(maxOpen)
	}
	var done atomic.Bool
	rs := &stats{}
	var wg sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for !done.Load() {
				t0 := time.Now()
				var err error
				if r%2 == 0 {
					var rows *sql.Rows
					rows, err = db.Query(`SELECT b.id,b.path,b.title,b.page_count,p.current_page FROM books b LEFT JOIN reading_progress p ON p.book_id=b.id WHERE b.library_id=? ORDER BY b.path LIMIT 500`, "L")
					if err == nil {
						for rows.Next() {
						}
						err = rows.Err()
						rows.Close()
					}
				} else {
					var n int
					err = db.QueryRow(`SELECT count(*) FROM books WHERE library_id=? AND missing=0`, "L").Scan(&n)
				}
				el := time.Since(t0)
				atomic.AddInt64(&rs.queries, 1)
				if err != nil {
					classify(rs, err)
				}
				rs.mu.Lock()
				rs.lat = append(rs.lat, el)
				rs.mu.Unlock()
			}
		}(r)
	}
	ws := &stats{}
	t0 := time.Now()
	func() {
		defer func() {
			if p := recover(); p != nil {
				classify(ws, fmt.Errorf("%v", p))
			}
		}()
		insertBatches(db, 0, 10000, 100, 1)
	}()
	wEl := time.Since(t0)
	done.Store(true)
	wg.Wait()
	db.Close()
	fmt.Printf("  %-40s writer 10000 rows/100 per tx: %s, writerErrs locked=%d busy=%d other=%d | readers: %d queries, locked=%d busy=%d other=%d, p50=%s p99=%s max=%s first=%q\n",
		label, wEl.Round(time.Millisecond), ws.locked, ws.busy, ws.other, rs.queries, rs.locked, rs.busy, rs.other,
		pct(rs.lat, .5).Round(time.Microsecond), pct(rs.lat, .99).Round(time.Microsecond), pct(rs.lat, 1).Round(time.Microsecond), rs.firstErr+ws.firstErr)
}

// ---- 4. dois escritores: lotes do scan + SaveProgress (ler-depois-escrever numa tx)
func benchTwoWriters(dir, label, d string, maxOpen int, mu, single bool) {
	useWriteMu = mu
	defer func() { useWriteMu = false }()
	db, _ := openFresh(dir, "two.db", d)
	if maxOpen > 0 {
		db.SetMaxOpenConns(maxOpen)
	}
	insertBatches(db, 0, 200, 200, 1) // livros que recebem progresso
	var done atomic.Bool
	ps := &stats{}
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			i := 0
			for !done.Load() {
				i++
				id := fmt.Sprintf("%032x", (i*7+w)%200)
				err := func() error {
					if single {
						// um unico UPSERT em autocommit; livro inexistente = erro de FK
						_, err := db.Exec(`INSERT INTO reading_progress(book_id,current_page,furthest_page,updated_at) VALUES(?,?,?,?)
ON CONFLICT(book_id) DO UPDATE SET current_page=excluded.current_page, furthest_page=max(furthest_page, excluded.furthest_page), updated_at=excluded.updated_at`, id, i%30, i%30, i)
						return err
					}
					if mu {
						writeMu.Lock()
						defer writeMu.Unlock()
					}
					tx, err := db.Begin()
					if err != nil {
						return err
					}
					defer tx.Rollback()
					var exists int
					if err := tx.QueryRow(`SELECT count(*) FROM books WHERE id=?`, id).Scan(&exists); err != nil {
						return err
					}
					if exists == 0 {
						return errors.New("not found")
					}
					if _, err := tx.Exec(`INSERT INTO reading_progress(book_id,current_page,furthest_page,updated_at) VALUES(?,?,?,?)
ON CONFLICT(book_id) DO UPDATE SET current_page=excluded.current_page, furthest_page=max(furthest_page, excluded.furthest_page), updated_at=excluded.updated_at`, id, i%30, i%30, i); err != nil {
						return err
					}
					return tx.Commit()
				}()
				atomic.AddInt64(&ps.queries, 1)
				if err != nil {
					classify(ps, err)
				}
				time.Sleep(2 * time.Millisecond)
			}
		}(w)
	}
	ws := &stats{}
	t0 := time.Now()
	func() {
		defer func() {
			if p := recover(); p != nil {
				classify(ws, fmt.Errorf("%v", p))
			}
		}()
		insertBatches(db, 1000, 10000, 100, 1)
	}()
	wEl := time.Since(t0)
	done.Store(true)
	wg.Wait()
	db.Close()
	fmt.Printf("  %-40s scan writer: %s errs(locked=%d busy=%d other=%d) | 2 progress writers (BEGIN; SELECT; UPSERT; COMMIT): %d tx, locked=%d busy=%d other=%d first=%q\n",
		label, wEl.Round(time.Millisecond), ws.locked, ws.busy, ws.other, ps.queries, ps.locked, ps.busy, ps.other, ps.firstErr+ws.firstErr)
}

func main() {
	dir := flag.String("dir", "", "pasta de trabalho (fora do repo)")
	flag.Parse()
	if *dir == "" {
		fmt.Println("-dir obrigatorio")
		os.Exit(2)
	}
	must(os.MkdirAll(*dir, 0o755))
	checkDSN(*dir)
	benchBatches(*dir)
	fmt.Println("== 3. 4 readers loop while 1 writer inserts")
	for run := 1; run <= 2; run++ {
		benchConcurrent(*dir, fmt.Sprintf("run%d free pool + busy_timeout", run), "", 0)
		benchConcurrent(*dir, fmt.Sprintf("run%d SetMaxOpenConns(1) + busy_timeout", run), "", 1)
		benchConcurrent(*dir, fmt.Sprintf("run%d free pool, NO busy_timeout", run), "file:{path}?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", 0)
	}
	fmt.Println("== 4. two writers (scan batches + progress tx)")
	for run := 1; run <= 2; run++ {
		benchTwoWriters(*dir, fmt.Sprintf("run%d deferred tx (default)", run), "", 0, false, false)
		benchTwoWriters(*dir, fmt.Sprintf("run%d _txlock=immediate", run), dsn("{path}", "&_txlock=immediate"), 0, false, false)
		benchTwoWriters(*dir, fmt.Sprintf("run%d SetMaxOpenConns(1)", run), "", 1, false, false)
		benchTwoWriters(*dir, fmt.Sprintf("run%d Go write mutex, deferred", run), "", 0, true, false)
		benchTwoWriters(*dir, fmt.Sprintf("run%d progress=single autocommit UPSERT", run), "", 0, false, true)
	}
}
