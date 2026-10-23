import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Until the custom domain is set up the site lives at johnschult.github.io/yagi/.
// With a domain (yagi.johnschult.com) set `site` to it and drop `base`.
export default defineConfig({
  site: 'https://johnschult.github.io',
  base: '/yagi',
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
      sidebar: [{ label: 'Start here', items: [{ slug: 'index' }] }],
    }),
  ],
});
