# panel

Generic container component that provides visual chrome (border, title, sizing) around arbitrary content. Follows bubbletea conventions but is a passive wrapper — it does not handle keyboard input or Esc.

## Usage

Consumers embed `panel.Model` and delegate chrome rendering to it via `View(content string)`. The consumer is responsible for:

- Opening/closing the panel (`SetActive`)
- Rendering inner content as a string
- Handling Esc and other key events

## Messages

- `PanelSetSizeMsg{PanelID, Width, Height}` — resize
- `PanelClosedMsg{PanelID}` — emitted by consumers when they close the panel (not by the panel itself)
