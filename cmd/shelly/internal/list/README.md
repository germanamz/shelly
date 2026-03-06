# list

Generic vertical list component that renders items with status icons, scrolling, and optional indentation. Supports two modes:

- **Read-only** (`selectable=false`): scroll only, no cursor — used by task browser
- **Selectable** (`selectable=true`): cursor navigation + item selection — used by sub-agent browser

## Usage

Create with `New(panelID, selectable)`. Update items with `SetItems()`. Render with `View()` or per-line with `RenderLine(index)`.

Consumers wrap the list output in a `panel.Model` for chrome, or use directly.

## Messages

- `ListItemSelectedMsg{PanelID, ItemID}` — user selected an item (selectable mode only)
- `ListDeactivatedMsg{}` — user pressed Esc (selectable mode only)
- `ListSetItemsMsg{Items}` — replace item list
- `ListSetSizeMsg{Width, Height}` — resize
