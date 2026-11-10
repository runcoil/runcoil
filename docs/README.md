# Runcoil docs site

The source of [docs.runcoil.dev](https://docs.runcoil.dev). The site is built with [Nimbus](https://github.com/cloudflare/nimbus), the docs framework of Cloudflare, on Astro. This file explains how to run and edit the site.

Agents read [AGENTS.md](AGENTS.md) first. It has the rules of a Nimbus site and the commands that check one.

## Run it

You need Node.js 26 or later and pnpm.

```sh
pnpm install
pnpm dev        # the site with live reload, without search
pnpm build      # the static site in dist/, with the search index
pnpm preview    # serve dist/, with search
```

Check the site before you open a pull request:

```sh
pnpm typecheck
pnpm build && pnpm check
```

`pnpm check` runs the lint rules too, and needs the build. `Docs CI` runs the same commands. `Docs Release` deploys `main` to Cloudflare.

## Write a page

- Describe only what `main` does. Create a page when its feature merges.
- A page is an `.mdx` file with a `title` and a `description` in its frontmatter. The title becomes the heading of the page, so the body has no `# heading`. The description is the line of the page in `llms.txt`.
- Set `sidebar.order` in the frontmatter to place the page in its section.
- End an internal link with a slash: `/concepts/steps/`.
- A callout is `:::note`, `:::tip`, `:::caution`, or `:::danger`, closed with `:::`.
- Use one complete example per page, with the expected output in a comment.

A diagram is Mermaid source in a `pre` element. The words around it must explain the same thing, because agents read the Markdown copy:

```mdx
<pre class="mermaid">{`
sequenceDiagram
    Worker->>Server: Listen for Runs
`}</pre>
```

## Agent copies

The build writes what agents read next to the HTML: `llms.txt`, one `llms.txt` per section, `llms-full.txt`, and an `index.md` copy of every page. Every page also links to its Markdown copy in its `<head>`.

## Change the look

Change the site in this order, and stop at the first step that works:

1. A setting in `astro.config.ts`.
2. A token in `src/styles/globals.css`.
3. A component from the Nimbus registry: `pnpm exec nimbus-docs add <name>`.
4. An edit to a copied file.

Do not write a new component when Nimbus has one. These copied files differ from the Nimbus originals, so keep their changes when you upgrade Nimbus:

| File | Change |
| --- | --- |
| `src/styles/globals.css` | Brand green, and softer dark colors |
| `src/components/Header.astro`, `src/layouts/DocsLayout.astro` | The Runcoil lockup and the Docs chip |
| `src/components/ui/pagination/` | A plain previous and next row |
| `src/pages/og/_card.ts` | The social card shows the lockup. `Logo.astro` and `logo.ts` are gone |
| `src/scripts/mermaid.ts` | Diagram lines take the brand color |
