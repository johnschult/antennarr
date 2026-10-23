import { defineConfig } from 'astro/config';
import { unified } from '@astrojs/markdown-remark';
import starlight from '@astrojs/starlight';
import rehypeDocsLinks from './plugins/rehype-docs-links.mjs';

const base = '/yagi';

// Until the custom domain is set up the site lives at johnschult.github.io/yagi/.
// With a domain (yagi.johnschult.com) set `site` to it and drop `base`.
export default defineConfig({
  site: 'https://johnschult.github.io',
  base,
  markdown: { processor: unified({ rehypePlugins: [[rehypeDocsLinks, { base }]] }) },
  integrations: [
    starlight({
      title: 'Yagi',
      description: 'Share an HDHomeRun tuner with the Plex, Jellyfin and Emby of family and friends.',
      logo: {
        light: './src/assets/yagi-wordmark-light.png',
        dark: './src/assets/yagi-wordmark-dark.png',
        alt: 'Yagi',
        replacesTitle: true,
      },
      favicon: '/favicon.svg',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/johnschult/yagi' }],
      customCss: ['./src/styles/theme.css'],
      components: { SocialIcons: './src/components/SocialIcons.astro' },
      routeMiddleware: './src/routeData.ts',
      sidebar: [
        {
          label: 'Use Yagi',
          items: [
            'getting-started',
            'web-ui',
            'networking',
            'media-servers',
            'tailscale',
            'configuration',
            'security',
            'troubleshooting',
            'faq',
          ],
        },
        { label: 'Build on it', items: ['api', 'roadmap', 'releasing'] },
      ],
    }),
  ],
});
