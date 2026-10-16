// Runs the converter (WebAssembly built from the Go sources) off the page's thread. Nothing here talks to a network except loading convert.wasm.
importScripts('wasm_exec.js');

let ready = null;
function boot() {
  if (ready) return ready;
  ready = (async () => {
    const go = new Go();
    const resp = await fetch('convert.wasm');
    const { instance } = await WebAssembly.instantiateStreaming(resp, go.importObject).catch(async () => {
      const buf = await (await fetch('convert.wasm')).arrayBuffer();
      return WebAssembly.instantiate(buf, go.importObject);
    });
    go.run(instance);                       // never returns: it serves pw64Convert and bgmConvert
    await new Promise(r => setTimeout(r, 0));
  })();
  return ready;
}

const crcTable = (() => { const t = new Uint32Array(256); for (let n = 0; n < 256; n++) { let c = n; for (let k = 0; k < 8; k++) c = c & 1 ? 0xEDB88320 ^ (c >>> 1) : c >>> 1; t[n] = c >>> 0; } return t; })();
function crc32(b) { let c = 0xFFFFFFFF; for (let i = 0; i < b.length; i++) c = crcTable[(c ^ b[i]) & 255] ^ (c >>> 8); return (c ^ 0xFFFFFFFF) >>> 0; }

// a zip with the files stored as they are (the data is already dense), inside a folder
function makeZip(folder, files) {
  const enc = new TextEncoder(), parts = [], central = []; let off = 0;
  const u16 = n => new Uint8Array([n & 255, (n >> 8) & 255]), u32 = n => new Uint8Array([n & 255, (n >>> 8) & 255, (n >>> 16) & 255, (n >>> 24) & 255]);
  const cat = a => { let n = 0; a.forEach(x => n += x.length); const o = new Uint8Array(n); let p = 0; a.forEach(x => { o.set(x, p); p += x.length; }); return o; };
  const now = new Date(), dt = ((now.getHours() << 11) | (now.getMinutes() << 5) | (now.getSeconds() >> 1)), dd = (((now.getFullYear() - 1980) << 9) | ((now.getMonth() + 1) << 5) | now.getDate());
  for (const f of files) {
    const name = enc.encode(folder + '/' + f.name), crc = crc32(f.data);
    const local = cat([u32(0x04034b50), u16(20), u16(0x0800), u16(0), u16(dt), u16(dd), u32(crc), u32(f.data.length), u32(f.data.length), u16(name.length), u16(0), name]);
    parts.push(local, f.data);
    central.push(cat([u32(0x02014b50), u16(20), u16(20), u16(0x0800), u16(0), u16(dt), u16(dd), u32(crc), u32(f.data.length), u32(f.data.length), u16(name.length), u16(0), u16(0), u16(0), u16(0), u32(0), u32(off), name]));
    off += local.length + f.data.length;
  }
  const cd = cat(central);
  return cat([...parts, cd, u32(0x06054b50), u16(0), u16(0), u16(files.length), u16(files.length), u32(cd.length), u32(off), u16(0)]);
}

const log = text => postMessage({ type: 'log', text });

onmessage = async (e) => {
  const m = e.data;
  try {
    await boot();
    if (m.kind === 'rom') {
      log('Reading the ROM...');
      const r = self.pw64Convert(new Uint8Array(m.bytes), !!m.allMusic, !!m.noMusic, log);
      if (r.error) { postMessage({ type: 'error', text: r.error }); return; }
      const files = []; for (let i = 0; i < r.files.length; i++) files.push({ name: r.files[i].name, data: r.files[i].data });
      const zip = makeZip('pw64vec', files);
      postMessage({ type: 'done', zip, name: 'pw64vec.zip', count: files.length }, [zip.buffer]);
    } else if (m.kind === 'mp3') {
      const files = [], seen = new Set(); let skipped = 0;
      for (let i = 0; i < m.files.length; i++) {
        const f = m.files[i];
        log(`(${i + 1}/${m.files.length}) ${f.name}`);
        const r = self.bgmConvert(m.game, f.name, new Uint8Array(f.buf));
        if (!r.known) { log('   not used: not one of the soundtrack file names'); skipped++; continue; }
        if (r.error) { log('   cannot read: ' + r.error); continue; }
        if (seen.has(r.nn)) { log('   track ' + r.nn + ' was already made from another copy: skipped'); continue; }
        seen.add(r.nn);
        const name = 'BGM' + String(r.nn).padStart(2, '0') + '.WAV';
        files.push({ name, data: r.wav });
        log(`   ${name}  ${r.secs.toFixed(1)} s  ${r.loops ? 'loops' : 'once'}`);
      }
      if (!files.length) { postMessage({ type: 'error', text: 'None of the files had a soundtrack name I know (for example "05. Hang Glider.mp3").' }); return; }
      files.sort((a, b) => a.name < b.name ? -1 : 1);
      const folder = m.game === 'mk' ? 'mk64vec' : 'pw64vec';
      const zip = makeZip(folder, files);
      postMessage({ type: 'done', zip, name: folder + '_music.zip', count: files.length, skipped }, [zip.buffer]);
    }
  } catch (err) { postMessage({ type: 'error', text: String(err && err.message || err) }); }
};
