// The docs are written to read well on GitHub too, so they link to each other as `other-page.md#heading`, to files
// beside the repository as `../README.md`, and to pictures as `screenshots/x.jpg`. This turns those into addresses that
// work on the site, whatever `base` is.
const REPO = 'https://github.com/johnschult/yagi/blob/main/';

export default function rehypeDocsLinks({ base = '/' } = {}) {
  const root = base.replace(/\/+$/, '');

  const page = (href) => {
    const m = /^(\.\.\/)?([^#?/]+)\.md(#.*)?$/.exec(href);
    if (!m) return href;
    const [, up, name, hash = ''] = m;
    return up ? `${REPO}${name}.md${hash}` : `${root}/${name.toLowerCase()}/${hash}`;
  };
  const asset = (src) => (/^(screenshots|brand)\//.test(src) ? `${root}/${src}` : src);

  // Pictures are written as HTML (for width and the dark/light pair), which arrives as raw text, not elements.
  const raw = (html) =>
    html
      .replace(/\b(src|srcset)="([^"]*)"/g, (_, attr, v) => `${attr}="${asset(v)}"`)
      .replace(/\bhref="([^"]*)"/g, (_, v) => `href="${page(v)}"`);

  const walk = (node) => {
    if (node.type === 'raw' && typeof node.value === 'string') node.value = raw(node.value);
    if (node.type === 'element') {
      const p = node.properties ?? {};
      if (node.tagName === 'a' && typeof p.href === 'string') p.href = page(p.href);
      if (node.tagName === 'img' && typeof p.src === 'string') p.src = asset(p.src);
      if (node.tagName === 'source' && typeof p.srcSet === 'string') p.srcSet = asset(p.srcSet);
    }
    node.children?.forEach(walk);
  };
  return (tree) => walk(tree);
}
