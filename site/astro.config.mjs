import { defineConfig } from 'astro/config';
import { unified } from '@astrojs/markdown-remark';
import starlight from '@astrojs/starlight';
import rehypeDocsLinks from './plugins/rehype-docs-links.mjs';

const base = '';

// The site is at the root of its own domain, so there is no base path. (Without a domain, on johnschult.github.io/yagi/,
// `base` would be '/yagi'.)
export default defineConfig({
  site: 'https://yagi.johnschult.com',
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
      components: {
        Hero: './src/components/Hero.astro',
        SocialIcons: './src/components/SocialIcons.astro',
      },
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
