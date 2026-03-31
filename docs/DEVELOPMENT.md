# Working on happygo

## Project structure

```
.
├─ common/
│  Meant for shared internal-use libraries for delve/,
│  tools/ and misc/. Ideally, we'd be able to figure
│  out a way to provide a minimal version that can also
│  be used inside go/, especially for the compiler
│  implementation and writing tests (not to be exposed
│  via stdlib APIs).
│
├─ docs/
│  Project-wide docs. Docs specific to go/ live alongside
│  its docs, not here.
│
├─ go/
│  Tracking golang/go. The Go compiler and standard library.
│
├─ delve/
│  Tracking go-delve/delve. The debugger.
│
├─ tools/
│  Tracking golang/tools. Supplementary tools such as gopls,
│  gofmt etc.
│
└─ misc/
   Our own internal tools etc. The top-level tools/ is already
   taken so. :/
```
