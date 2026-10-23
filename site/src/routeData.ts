import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { getRoadmap } from './lib/roadmap';

// The Roadmap page draws its headings in a component, which Starlight's "On this page" list doesn't see: add them.
export const onRequest = defineRouteMiddleware(async (context) => {
  const route = context.locals.starlightRoute;
  if (route.id !== 'roadmap') return;
  const roadmap = await getRoadmap();
  if (!roadmap) return;
  const items = roadmap.sections.map((s) => ({ depth: 2, slug: s.id, text: s.heading, children: [] }));
  if (roadmap.shipped.length) items.push({ depth: 2, slug: 'shipped', text: 'Recently shipped', children: [] });
  route.toc?.items.push(...items);
});
