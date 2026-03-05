# Rich Media Input Plan

Support pasting files or dragging them into the terminal so they are sent to agents and LLMs as multi-modal content parts.

## Current State

### What exists
- **Data model ready**: `content.Image` and `content.Document` types defined in `pkg/chats/content/content.go`
- **Session persistence ready**: `pkg/sessions/` has `FileAttachmentStore` with content-addressable storage, `MarshalMessagesWithAttachments`/`UnmarshalMessagesWithAttachments` for binary round-tripping (SHA-256 deduplication)
- **SendParts API ready**: `engine.Session.SendParts(ctx, ...content.Part)` accepts arbitrary parts
- **Bubbletea v2 paste events**: `tea.PasteMsg`, `tea.PasteStartMsg`, `tea.PasteEndMsg` exist in bubbletea v2
- **File picker**: `@` trigger walks the working directory, but only inserts file path as text

### What's missing
1. **TUI**: `InputSubmitMsg` only carries `Text string` — no content parts
2. **TUI**: No paste interception or file path resolution from pasted/dropped text
3. **Providers**: All four providers (Anthropic, OpenAI/Grok, Gemini) silently skip `Image` and `Document` parts
4. **Chat view**: No rendering for attached files/images
5. **App layer**: `handleSubmit` calls `sess.Send(text)` — never `SendParts`

## Design

### File detection strategy

Terminal drag-and-drop typically pastes the file's absolute path as text (quoted or unquoted). Combined with explicit `@` picker selection, there are two input paths:

1. **Explicit `@` selection** — file picker already resolves the path; enhance it to create attachment parts
2. **Paste detection** — intercept `tea.PasteMsg`, scan for file paths, offer to attach detected files

For pasted content, detect file paths matching:
- Absolute paths: `/path/to/file`, `~/path/to/file`
- Quoted paths from drag-and-drop: `'/path/to/file'`, `"/path/to/file"`
- Multiple paths (newline-separated or space-separated quoted)

MIME type detection via `net/http.DetectContentType` (stdlib) on the first 512 bytes, plus extension-based mapping for known types (`.pdf`, `.png`, `.jpg`, `.go`, `.md`, etc.).

### Supported file types

| Category | Extensions | Content Part | Notes |
|----------|-----------|-------------|-------|
| Images | png, jpg, jpeg, gif, webp | `content.Image` | Base64 to API |
| Documents | pdf | `content.Document` | Anthropic: document block; OpenAI: file search/content; Gemini: inline_data |
| Text/Code | go, md, txt, json, yaml, etc. | `content.Text` | Read file content, wrap as text with filename header |

Text/code files are the simplest — just read and inline as a text part with a header like `--- file: path/to/file.go ---`. No provider changes needed for these.

Binary files (images, PDFs) require provider-specific encoding.

### Size limits

- Max single file: 20 MB (Anthropic limit; others are similar)
- Max total attachments per message: 100 MB
- Files exceeding limits: show error in TUI, don't attach

---

## Implementation Phases

### Phase 1: TUI input plumbing ✅ COMPLETE

**Goal**: Carry file attachments from input through to `SendParts`.

#### 1a. Extend `InputSubmitMsg` to carry parts

`cmd/shelly/internal/msgs/msgs.go`:
```go
type InputSubmitMsg struct {
    Text  string
    Parts []content.Part // non-text parts (images, documents)
}
```

#### 1b. Track pending attachments in `InputModel`

`cmd/shelly/internal/input/input.go`:
```go
type InputModel struct {
    // ... existing fields
    attachments []Attachment // pending file attachments
}

type Attachment struct {
    Path      string
    Data      []byte
    MediaType string
    Kind      string // "image", "document", "text"
}
```

- On submit, convert `attachments` to `[]content.Part` and include in `InputSubmitMsg`
- Display attachment count/names in the input border or as a tag line below the textarea
- Add `Ctrl+U` or similar keybinding to clear all pending attachments
- Add `Ctrl+A` keybinding to open a file picker specifically for attaching

#### 1c. Enhance `@` file picker to attach files

When a file is selected via `@`, instead of inserting the path as text:
1. Read the file
2. Detect MIME type
3. Add to `attachments` list
4. Insert a visual indicator in the textarea (e.g., `[file.pdf]`) or display as a tag below

#### 1d. Intercept paste events for file paths

In `InputModel.Update`:
```go
case tea.PasteMsg:
    paths := detectFilePaths(string(msg))
    if len(paths) > 0 {
        // Read files, add to attachments
        // Insert remaining non-path text into textarea
    } else {
        // Normal paste — forward to textarea
    }
```

`detectFilePaths` scans for file-path patterns and verifies with `os.Stat`.

#### 1e. Update `handleSubmit` in app

`cmd/shelly/internal/app/app.go`:
```go
func (m *AppModel) handleSubmit(msg msgs.InputSubmitMsg) (tea.Model, tea.Cmd) {
    // ... existing command dispatch ...

    parts := msg.Parts
    if msg.Text != "" {
        parts = append([]content.Part{content.Text{Text: msg.Text}}, parts...)
    }

    // Use SendParts instead of Send
    _, err := sess.SendParts(sendCtx, parts...)
}
```

### Phase 2: Chat view rendering

**Goal**: Display attached files in the conversation view.

- Render image attachments as `[Image: filename.png (2.3 MB)]` with MIME type
- Render document attachments as `[Document: report.pdf (150 KB)]`
- Render text file attachments with filename header (already text, so can show content)
- Use distinct styling (dimmed, bordered) to differentiate from typed text

### Phase 3: Provider support for images

**Goal**: Send `content.Image` to LLM APIs.

#### 3a. Anthropic (`pkg/providers/anthropic/anthropic.go`)

Add to `partToBlock`:
```go
case content.Image:
    b64 := base64.StdEncoding.EncodeToString(v.Data)
    return &apiContent{
        Type: "image",
        Source: &apiSource{
            Type:      "base64",
            MediaType: v.MediaType,
            Data:      b64,
        },
    }
```

Add `apiSource` struct to the types.

#### 3b. OpenAI/Grok (`pkg/providers/internal/openaicompat/convert.go`)

OpenAI multi-modal messages use a content array instead of a string. Update `ConvertMessages`:
- For user messages with Image parts, switch from `Content: &text` to `ContentParts: [...]` array format
- Each image becomes `{"type": "image_url", "image_url": {"url": "data:image/png;base64,..."}}`

Add `ContentPart` types and update the `Message` struct to support both string and array content.

#### 3c. Gemini (`pkg/providers/gemini/gemini.go`)

Add to `partToAPIPart`:
```go
case content.Image:
    b64 := base64.StdEncoding.EncodeToString(v.Data)
    return &apiPart{
        InlineData: &apiBlob{
            MimeType: v.MediaType,
            Data:     b64,
        },
    }, nil
```

Add `apiBlob` and `InlineData` field to `apiPart`.

### Phase 4: Provider support for documents (PDF)

**Goal**: Send `content.Document` parts to LLM APIs.

#### 4a. Anthropic

Anthropic supports PDF documents as a `"document"` content block:
```go
case content.Document:
    b64 := base64.StdEncoding.EncodeToString(v.Data)
    return &apiContent{
        Type: "document",
        Source: &apiSource{
            Type:      "base64",
            MediaType: v.MediaType,
            Data:      b64,
        },
    }
```

#### 4b. OpenAI/Grok

OpenAI supports file content via `{"type": "file", "file": {"filename": "...", "file_data": "data:application/pdf;base64,..."}}` in the content array. Add this to the multi-modal message format from Phase 3b.

#### 4c. Gemini

Gemini supports PDF via `inline_data` (same as images):
```go
case content.Document:
    b64 := base64.StdEncoding.EncodeToString(v.Data)
    return &apiPart{
        InlineData: &apiBlob{
            MimeType: v.MediaType,
            Data:     b64,
        },
    }, nil
```

### Phase 5: Text file inlining (low priority)

For text/code files, no provider changes needed. At the TUI layer, read the file and create a `content.Text` part:

```go
content.Text{Text: fmt.Sprintf("--- file: %s ---\n%s\n--- end file ---", filename, fileContent)}
```

This is effectively what the `@` picker should do for non-binary files — inline the content as text so it works with all providers immediately.

---

## File structure (new/modified)

| File | Change |
|------|--------|
| `cmd/shelly/internal/msgs/msgs.go` | Add `Parts` field to `InputSubmitMsg` |
| `cmd/shelly/internal/input/input.go` | Add `attachments` field, paste interception, submit with parts |
| `cmd/shelly/internal/input/attachment.go` | **New** — `Attachment` type, file reading, MIME detection, path detection |
| `cmd/shelly/internal/input/filepicker.go` | Return attachment data instead of just path text |
| `cmd/shelly/internal/app/app.go` | Use `SendParts` in `handleSubmit`, display attachments in chat view |
| `cmd/shelly/internal/chatview/` | Render image/document attachment indicators |
| `pkg/providers/anthropic/anthropic.go` | Add Image and Document cases to `partToBlock`, add `apiSource` type |
| `pkg/providers/internal/openaicompat/convert.go` | Multi-modal content array support for Image and Document |
| `pkg/providers/internal/openaicompat/types.go` | Add `ContentPart`, `ImageURL`, `FileData` types |
| `pkg/providers/gemini/gemini.go` | Add Image and Document cases to `partToAPIPart`, add `apiBlob`/`InlineData` |

## Priority order

1. **Phase 5 (text file inlining)** — Highest immediate value, zero provider changes. Enhance `@` picker to read and inline text/code files as `content.Text`.
2. **Phase 1 (TUI plumbing)** — Required foundation for everything else.
3. **Phase 3 (image support)** — High value; all major providers support images.
4. **Phase 2 (chat view)** — Polish; users need feedback on what was attached.
5. **Phase 4 (document/PDF support)** — Lower priority; API support varies.

## Open questions

1. **Clipboard image paste**: Should we support pasting images from clipboard (not file paths)? Would require reading clipboard binary data via `pbpaste` on macOS or OSC 52. Deferred for now.
2. **File size UX**: Should large files show a confirmation prompt before attaching, or silently fail with an error message?
3. **Token estimation**: Should attached files contribute to the token counter display? Images use fixed token counts per provider; documents vary.
4. **Drag-and-drop in non-macOS terminals**: Linux terminals may paste paths differently (e.g., `file:///path/to/file` URIs). Worth testing.
