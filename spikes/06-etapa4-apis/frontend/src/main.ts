// SPIKE (Etapa 4, Task 1) — descartavel. Tudo que o frontend observa vai
// para o log do Go via SpikeService.Report, para ser capturado como evidencia.
import { Events } from "@wailsio/runtime";
import { SpikeService } from "../bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/spikes/06-etapa4-apis";

function out(id: string, text: string) {
    document.getElementById(id)!.textContent = text;
    SpikeService.Report(`${id} ${text}`);
}

function describeErr(err: unknown): string {
    const e = err as any;
    let json = "";
    try { json = JSON.stringify(e); } catch { json = "<unserializable>"; }
    return `ctor=${e?.constructor?.name} name=${e?.name} message=${JSON.stringify(e?.message)} cause=${JSON.stringify(e?.cause)} json=${json}`;
}

// ---- (b)/(d): evento emitido dentro de ServiceStartup chega?
let startupSeen = false;
Events.On("startup-ping", () => { startupSeen = true; });
setTimeout(() => out("startup", `startup-ping seen=${startupSeen}`), 1500);

// ---- (a) dialogo de pasta
document.getElementById("pick")!.addEventListener("click", async () => {
    const t0 = performance.now();
    try {
        const p = await SpikeService.PickFolder();
        out("pickout", `pick RESOLVED value=${JSON.stringify(p)} typeof=${typeof p} after=${Math.round(performance.now() - t0)}ms`);
    } catch (err) {
        out("pickout", `pick REJECTED ${describeErr(err)} after=${Math.round(performance.now() - t0)}ms`);
    }
});

// ---- (b) burst de eventos
let received = 0;
let firstShape = "";
let dupes = 0;
let perWorkerOutOfOrder = 0;
let globalOutOfOrder = 0;
let lastGlobal = 0;
let tFirst = 0;
let tLast = 0;
let doneSeenAtCount = -1;
const seenGlobal = new Set<number>();
const lastSeqByWorker = new Map<number, number>();

Events.On("burst", (e) => {
    const now = performance.now();
    if (received === 0) {
        tFirst = now;
        firstShape = JSON.stringify({ keys: Object.keys(e), ctor: (e as any)?.constructor?.name, name: e.name, sender: (e as any).sender, data: e.data, typeofData: typeof e.data });
    }
    received++;
    tLast = now;
    const d = e.data; // tipado como BurstEvent via RegisterEvent + eventdata.d.ts
    if (seenGlobal.has(d.Global)) dupes++;
    seenGlobal.add(d.Global);
    const prev = lastSeqByWorker.get(d.Worker);
    if (prev !== undefined && d.Seq !== prev + 1) perWorkerOutOfOrder++;
    lastSeqByWorker.set(d.Worker, d.Seq);
    if (d.Global < lastGlobal) globalOutOfOrder++;
    lastGlobal = Math.max(lastGlobal, d.Global);
});

Events.On("burst-done", (e) => {
    doneSeenAtCount = received;
    // espera mais 2 s para pegar retardatarios depois do "done"
    setTimeout(() => {
        const missing: number[] = [];
        for (let g = 1; g <= (e.data as number); g++) if (!seenGlobal.has(g)) missing.push(g);
        out("burstout", `burst received=${received} expected=${e.data} unique=${seenGlobal.size} dupes=${dupes} missing=${missing.length} perWorkerOutOfOrder=${perWorkerOutOfOrder} globalOutOfOrder(vs emit counter)=${globalOutOfOrder} doneArrivedAfter=${doneSeenAtCount} spanFirstToLast=${Math.round(tLast - tFirst)}ms workers=${lastSeqByWorker.size}`);
        out("burstout", `burst firstEventShape=${firstShape}`);
    }, 2000);
});

document.getElementById("burst")!.addEventListener("click", async () => {
    const t0 = performance.now();
    const n = await SpikeService.EmitBurst(1000);
    SpikeService.Report(`burstcall EmitBurst(1000) resolved=${n} after=${Math.round(performance.now() - t0)}ms receivedSoFar=${received}`);
});

// ---- (c) formato do retorno
document.getElementById("sample")!.addEventListener("click", async () => {
    const r = await SpikeService.Sample();
    const anyR = r as any;
    out("sampleout", `sample json=${JSON.stringify(r)}`);
    out("sampleout", `sample ctor=${anyR?.constructor?.name} keys=${Object.keys(r).join(",")} typeofID=${typeof r.ID} ID===2^53+1? ${r.ID === 9007199254740993} ID.toString=${String(r.ID)} InnerPtr===null? ${r.InnerPtr === null} NilItems=${JSON.stringify(r.NilItems)} hasOmitted=${"omittedField" in anyR} hasUnexported=${"unexported" in anyR}`);
});

// ---- chamadas concorrentes: Slow(1500) em paralelo com Sample()
document.getElementById("par")!.addEventListener("click", async () => {
    const t0 = performance.now();
    let tSlow = 0, tSample = 0;
    const p1 = SpikeService.Slow(1500).then(() => { tSlow = performance.now() - t0; });
    const p2 = SpikeService.Sample().then(() => { tSample = performance.now() - t0; });
    await Promise.all([p1, p2]);
    out("parout", `par Sample resolved at ${Math.round(tSample)}ms, Slow(1500) resolved at ${Math.round(tSlow)}ms -> concurrent=${tSample < tSlow}`);
});
