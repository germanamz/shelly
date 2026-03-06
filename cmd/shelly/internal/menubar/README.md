# menubar

Generic horizontal menu bar component following bubbletea conventions. Renders as a single line of items separated by dividers. Not coupled to any specific feature.

## Usage

Create with `New()`. Add items with `AddOrUpdateItem()` or replace all with `SetItems()`. Control visibility with `SetVisible()` and focus with `SetActive()`.

The menu bar is hidden by default and takes zero layout space until `SetVisible(true)` is called.

## Messages

- `MenuItemSelectedMsg{ID}` — user activated an item (Enter/Space)
- `MenuDeactivatedMsg{}` — user pressed Esc, menu released focus
- `MenuSetItemsMsg{Items}` — replace item list

## Key Handling

The consumer is responsible for forwarding key events. The model exposes `MoveLeft()`, `MoveRight()`, and `Select()` methods for the consumer to call based on key presses.
