// The roadmap is the open issues labeled `roadmap` in the repository, grouped by milestone, and the closed ones as
// shipped. It is read from GitHub when the site is built; the site is rebuilt whenever an issue or milestone changes
// (see .github/workflows/site.yml).
export const REPO = 'johnschult/yagi';
export const ISSUES = `https://github.com/${REPO}/issues`;

export interface Issue {
  number: number;
  title: string;
  body: string | null;
  html_url: string;
  state: 'open' | 'closed';
  closed_at: string | null;
  milestone: { number: number; title: string; description: string | null } | null;
  pull_request?: unknown;
}

export interface Section {
  id: string;
  heading: string;
  description: string | null;
  items: Issue[];
}

export interface Roadmap {
  sections: Section[];
  shipped: Issue[];
}

const slug = (t: string) => t.toLowerCase().replace(/[^a-z0-9]+/g, '-');
const rank = (t: string) => (t === 'Later' ? 2 : t === 'Not scheduled' ? 3 : 1);

async function load(): Promise<Roadmap | null> {
  let issues: Issue[];
  try {
    const headers: Record<string, string> = { Accept: 'application/vnd.github+json' };
    if (process.env.GITHUB_TOKEN) headers.Authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
    const res = await fetch(`https://api.github.com/repos/${REPO}/issues?labels=roadmap&state=all&per_page=100`, { headers });
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    issues = ((await res.json()) as Issue[]).filter((i) => !i.pull_request);
  } catch (err) {
    console.warn(`[roadmap] could not read the issues: ${err}`);
    return null;
  }

  // Milestones in order (versions first, "Later" then "Not scheduled" last), each with its open issues.
  const groups = new Map<string, { description: string | null; items: Issue[] }>();
  for (const i of issues.filter((i) => i.state === 'open')) {
    const key = i.milestone?.title ?? 'Not scheduled';
    if (!groups.has(key)) groups.set(key, { description: i.milestone?.description ?? null, items: [] });
    groups.get(key)!.items.push(i);
  }
  const sections = [...groups.entries()]
    .sort((a, b) => rank(a[0]) - rank(b[0]) || a[0].localeCompare(b[0], undefined, { numeric: true }))
    .map(([title, { description, items }]) => ({
      id: slug(title),
      heading: title === 'Later' || title === 'Not scheduled' ? title : `Planned for ${title}`,
      description,
      items,
    }));

  const shipped = issues
    .filter((i) => i.state === 'closed')
    .sort((a, b) => (b.closed_at ?? '').localeCompare(a.closed_at ?? ''))
    .slice(0, 10);
  return { sections, shipped };
}

// One read per build, shared by the page and its table of contents.
let cached: Promise<Roadmap | null> | undefined;
export const getRoadmap = () => (cached ??= load());
