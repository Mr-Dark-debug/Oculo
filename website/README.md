# Oculo Premium Website

The official website for Oculo, designed for the "Glass Box" experience. Built with modern UI/UX principles, focusing on clarity, depth, and performance.

## Design System

- **Aesthetic:** Refined Glass / Bento Grid.
- **Palette:** Pure white base with Indigo and Sky accents.
- **Typography:** Plus Jakarta Sans (Body), Space Grotesk (Headings), JetBrains Mono (Code).
- **Interactions:** Magnetic buttons, staggered scroll reveals, typing terminal.

## Key Files

- `index.html`: Main entry (Root).
- `website/assets/css/styles.css`: Advanced design system and layout logic.
- `website/assets/js/main.js`: Interactivity, component loader, and GitHub API integration.
- `website/components/`: Modular HTML templates for a scalable architecture.

## Development

To view the website locally, you **must** serve it through a web server to allow dynamic component loading via the `fetch` API.

```bash
# Using Python
python -m http.server 8000

# Using Node.js
npx serve .
```

Visit `http://localhost:8000`.

## Deployment

Recommended deployment platforms:
- **GitHub Pages:** Deploy from the `main` branch.
- **Vercel / Netlify:** Connect the repository and deploy with default settings (no build step).

## Credits

Oculo is an open-source project dedicated to AI transparency.
https://github.com/Mr-Dark-debug/Oculo
