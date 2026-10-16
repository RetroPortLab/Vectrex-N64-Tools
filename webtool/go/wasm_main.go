//go:build js && wasm

package main

import "syscall/js"

// the page's two calls (run in a Web Worker): everything happens in the browser, nothing is uploaded.
//   pw64Convert(romBytes Uint8Array, allMusic bool, noMusic bool, say function) -> {error, files:[{name, data}]}
//   bgmConvert(game string, fileName string, mp3 Uint8Array) -> {known, error, nn, loops, secs, wav}

func toBytes(v js.Value) []byte {
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func toJS(b []byte) js.Value {
	a := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(a, b)
	return a
}

func main() {
	js.Global().Set("pw64Convert", js.FuncOf(func(this js.Value, a []js.Value) any {
		res := js.Global().Get("Object").New()
		say := func(s string) { a[3].Invoke(s) }
		files, err := convertROM(toBytes(a[0]), a[1].Bool(), a[2].Bool(), say)
		if err != nil {
			res.Set("error", err.Error())
			return res
		}
		arr := js.Global().Get("Array").New()
		for _, f := range files {
			o := js.Global().Get("Object").New()
			o.Set("name", f.Name)
			o.Set("data", toJS(f.Data))
			arr.Call("push", o)
		}
		res.Set("files", arr)
		return res
	}))
	js.Global().Set("bgmConvert", js.FuncOf(func(this js.Value, a []js.Value) any {
		res := js.Global().Get("Object").New()
		nn, wav, secs, loops, known, err := convertMP3(a[0].String(), a[1].String(), toBytes(a[2]))
		res.Set("known", known)
		if err != nil {
			res.Set("error", err.Error())
			return res
		}
		if known {
			res.Set("nn", nn)
			res.Set("loops", loops)
			res.Set("secs", secs)
			res.Set("wav", toJS(wav))
		}
		return res
	}))
	js.Global().Get("self").Call("postMessage", map[string]any{"type": "ready"})
	select {}
}
