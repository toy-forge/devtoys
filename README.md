# DevToys TUI

A developer toolbox for the terminal. It brings the tools of [DevToys](https://github.com/veler/DevToys), a desktop toolbox for developers, into a full-screen TUI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

- 31 tools in seven categories: converters, encoders, formatters, generators, graphic, testers, and text.
- Most tools compute their output as you type.
- Mouse and keyboard both work. Press `/` to search the tool list.
- The UI is in English. Sidebar icons use Nerd Font glyphs.

This is an independent terminal re-implementation. It is not an official DevToys project.

## Requirements

- Go 1.27.3 or later (see `go.mod`)
- A terminal with mouse support and a Nerd Font for the sidebar icons
- Optional: [Ghostty](https://ghostty.org) or [Kitty](https://sw.kovidgoyal.net/kitty/) for native image previews

## Running

```bash
# Run directly in full-screen mode
go run .

# Or build a binary and run it
go build -o devtoys .
./devtoys
```

Press `ctrl+c` to quit.

## Usage

### Navigating

- The sidebar on the left lists the categories. The list on the right shows the tools in the selected category.
- `tab` moves focus between the sidebar and the tool list.
- `/` starts a search over tool titles and descriptions. `enter` keeps the filter, and `esc` clears it.
- `enter` opens the selected tool. `esc` returns to the list.

### Working in a tool

- Each tool shows its key hints in the status bar at the bottom.
- `tab` and `shift+tab` move focus between the input and output panes. Some tools have three panes and cycle through them.
- `ctrl+y` copies the output to the clipboard. ANSI color codes are stripped before copying.
- Most tools support the mouse: click to place the cursor in an input, click buttons, and scroll the wheel over inputs and outputs.

### Opening files

Press `ctrl+f`, or click **Open File**, in a tool that accepts files:

- UTF-8 text files of up to 1 MiB are inserted into the input.
- PNG, JPEG, and GIF images of up to 10 MiB and 16 megapixels can be previewed.
- Tabs and carriage returns are shown as `␉` and `␍` and kept in calculations. If the text already contains these markers, the escape prefix `␛` is shown so the two cannot be confused. Other control characters are rejected when a file is opened.

### Image preview

In Ghostty or Kitty, press `i` on an image to view it through the Kitty graphics protocol, and press `enter` to return. Other terminals fall back to a character-based preview. Native preview is not enabled inside tmux.

### Generator option bar

Password, UUID, and Lorem Ipsum share an option bar:

- `ctrl+o` focuses or leaves the bar.
- `+` / `-` adjust numeric options, `←` / `→` switch choices, and `1`–`9` toggle switches in order. `tab` leaves the bar.
- `alt+r` regenerates the output.

## Tools

### Converters

| Tool | Description |
|---|---|
| Cron expression parser | Parses cron expressions, including an optional seconds field, and lists upcoming run times. |
| Date & Time | Converts between epoch timestamps (seconds, milliseconds, custom epochs, .NET ticks) and ISO 8601 date and time fields. Supports IANA time zones and daylight-saving information. |
| Number Base Converter | Converts between hexadecimal, decimal, octal, and binary. Advanced mode adds RFC 4648 and custom alphabets, bases 2–36, and large integers. |
| JSON <> YAML Converter | Converts in either direction, with automatic or manual detection and 2- or 4-space indentation. |
| JSON Array to Table | Flattens a JSON array of objects into a table and copies it as delimited text. |

### Encoders

| Tool | Description |
|---|---|
| Base64 Text Encoder / Decoder | Encodes and decodes UTF-8 or ASCII text, with standard or URL-safe alphabets and optional padding. |
| Base64 Image Encoder / Decoder | Converts between image files and Base64 data. |
| Certificate Decoder | Decodes PEM and DER X.509 certificates and password-protected PKCS#12 files locally. |
| GZip Compress / Decompress | Compresses text to GZip and then Base64, or reverses the process. |
| HTML Text Encoder / Decoder | Escapes and unescapes HTML entities. |
| JWT Encoder / Decoder | Encodes, decodes, and verifies JWTs with HS, RS, PS, and ES algorithms (256/384/512 bit). Accepts text or Base64 HMAC keys and PEM keys, and checks standard claims. |
| QR Code Encoder / Decoder | Renders QR codes in the terminal with half-block characters and adjustable error correction. Decodes QR codes from images. |
| URL Encoder / Decoder | Encodes and decodes either a single component or a full URL, with the URL broken into its parts. |

### Formatters

| Tool | Description |
|---|---|
| JSON Formatter | Formats, minifies, and sorts JSON properties, with syntax highlighting. |
| XML Formatter | Pretty-prints or minifies XML and checks that it is well formed. |
| SQL Formatter | Formats SQL in 10 dialects: Standard SQL, DB2, MariaDB, MySQL, N1QL, PL/SQL, PostgreSQL, Redshift, Spark SQL, and Transact-SQL. |

### Generators

| Tool | Description |
|---|---|
| Hash and Checksum Generator | Computes MD5, SHA-1, SHA-256/384/512, CRC32, and HMAC. Hashes files of any size in the background and compares against an expected value. |
| UUID Generator | Generates UUIDs in batches of up to 10,000, in version 1, 4, or 7, with uppercase and hyphen options. |
| Password Generator | Generates up to 10,000 passwords of 5–128 characters (default 30) from uppercase, lowercase, digit, and special-character sets, using `crypto/rand`. Characters typed in the input are excluded. |
| Lorem Ipsum Generator | Generates paragraphs, sentences, or words from the classic Lorem Ipsum corpus. |

### Graphic

| Tool | Description |
|---|---|
| Color Blindness Simulator | Previews an image alongside protanopia, deuteranopia, and tritanopia simulations. Saves the selected result as PNG. |
| Image Converter | Converts images in batches between PNG, JPEG, GIF, BMP, TIFF, PBM, TGA, and lossless WebP, with a queue. |

### Testers

| Tool | Description |
|---|---|
| Regular Expression Tester | Tests .NET-style regular expressions (lookaround, backreferences, match options) with live highlighting and a match timeout. |
| JSONPath Tester | Runs JSONPath queries against a JSON document and lists every match. |
| XPath | Runs XPath queries against an XML document and lists every matching node. |
| XML / XSD Tester | Validates XML against an XSD 1.0 schema. External entities and remote schemas are disabled by default. |

### Text

| Tool | Description |
|---|---|
| Text Analyser and Utilities | Counts bytes, characters, words, sentences, paragraphs, and line-break types. Shows word and character frequencies, changes case, and sorts lines. |
| Text Comparer | Diffs two texts line by line, in a side-by-side or unified view. |
| Text Escape / Unescape | Escapes and unescapes common string escape sequences. |
| List Comparer | Computes the intersection, union, and both difference sets of two lists. |
| Markdown Preview | Edits Markdown on the left and previews it on the right. Supports tables, task lists, footnotes, emoji, smart punctuation, and YAML front matter. |

## Reference sources

The TUI is modeled on DevToys, a desktop toolbox for developers:

- **[DevToys](https://github.com/veler/DevToys)** is the reference for the category sidebar, the tool list home page, the tool page headers, and the behavior of each tool. The TUI implements a subset of its tools.
- **Lorem Ipsum corpus.** The corpus and its MIT license are copied from DevToys's C# implementation, which forked the NLipsum corpus by Alex Pendleton. See `internal/tools/generators/lorem/corpora/LICENSE.MD`.
- **[sql-formatter](https://github.com/sql-formatter-org/sql-formatter) 4.0.2** (MIT) is bundled in `internal/tools/formatters/sqlfmt/sqlformatter/` and run through [goja](https://github.com/dop251/goja). Its leading-comma handler is adapted to match DevToys. See the NOTICE file in that directory.
- **libDaltonLens** (Unlicense). The Brettel 1997 coefficients in the Color Blindness Simulator come from it, and DevToys uses the same coefficients. See `internal/tools/graphic/colorblind/DALTONLENS-LICENSE`.
- **Bubbles textarea** (MIT). `internal/toolkit/editor/textarea` is a fork of the multi-line input from [Bubbles](https://github.com/charmbracelet/bubbles). See its LICENSE file.
- **GitHub Dark and GitHub Light** color palettes, defined in `internal/theme/colors.go`.
- **Standards.** Encodings follow RFC 4648. Date formats follow ISO 8601 and RFC 3339. Certificates and keys follow X.509 and PKCS#12. XML validation follows XSD 1.0.

The main third-party dependencies are Bubble Tea, Bubbles, Lip Gloss, Chroma, Goldmark, goja, regexp2, ojg, xmlquery, go-qrcode, gozxing, go-jwt, go-pkcs12, and nativewebp. See `go.mod` for the full list.

## Project layout

```
.
├── main.go                  # entry point: imports internal/tools, runs the full-screen program
└── internal/
    ├── app/                 # shell: sidebar, tool list, tool pages, status bar, file opening
    ├── theme/               # colors, styles, icons
    ├── ui/                  # component library: buttons, toolbars, selects, tables, modals, toasts
    ├── imaging/             # vector image (SVG) decoding for image tools
    ├── toolkit/             # Tool interface, registry, categories, shared workspace, settings bar
    │   ├── editor/          #   text input that keeps tabs and newlines
    │   │   └── textarea/    #   fork of the Bubbles textarea (MIT)
    │   ├── filepicker/      #   clickable file picker
    │   └── imageview/       #   terminal image preview (Kitty protocol with fallback)
    └── tools/
        ├── tools.go         # blank imports that trigger tool registration
        ├── converters/      # cronparser, dateparser, jsontable, jsonyaml, numberbase
        ├── encoders/        # base64, base64image, certificate, gzip, htmlenc, jwt, qrcode, urlenc
        ├── formatters/      # jsonfmt, sqlfmt, xmlfmt
        ├── generators/      # hash, lorem, password, uuid, internal/optbar
        ├── graphic/         # colorblind, imageconvert
        ├── testers/         # jsonpath, regex, xmlvalidator, xpath
        └── text/            # analyzer, comparer, escape, listcompare, markdown
```

## Adding a tool

1. Create a package under `internal/tools/<category>/<name>/` that implements `toolkit.Tool`. The `ID()` method returns a value such as `encoders.base64`. The model should respond to `tea.WindowSizeMsg` so the layout adapts to the terminal size.
2. Call `toolkit.Register(...)` from the package's `init()` function.
3. Add a blank import of the package to `internal/tools/tools.go`.

## Development

```bash
go vet ./...
go test ./...
```

Each tool package has its own tests.

## License

This repository does not include a top-level license file. Bundled third-party code keeps its own license, as noted in the reference sources above.
