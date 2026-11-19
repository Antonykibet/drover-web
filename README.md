# Drover Profit — landing page

Static marketing / waitlist page for **Drover Profit**: project profitability for design studios.

## Structure

```
index.html        # the whole page: design tokens, components, markup, waitlist script
shots/            # photography and product screenshots used on the page
```

No build step. Open it locally with any static server:

```sh
python3 -m http.server 8000
# then visit http://localhost:8000
```

## Notes

- All styling resolves to CSS custom properties defined in `:root` (the "Drover design system v2").
- The waitlist form currently writes through the Claude artifact runtime (`window.claude.use('db')`).
  Outside that runtime it shows a "nothing was saved" notice — wire it to a real backend before launch.
