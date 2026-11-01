/**
 * plugins/vuetify.ts
 *
 * Framework documentation: https://vuetifyjs.com`
 */

// Styles
import '@mdi/font/css/materialdesignicons.css'
import 'vuetify/styles/main.css'
import '@/styles/obhod.css'

import { fa, en, vi, zhHans, zhHant, ru } from 'vuetify/locale'

// Composables
import { createVuetify } from 'vuetify'

// https://vuetifyjs.com/en/introduction/why-vuetify/#feature-guides
export default createVuetify({
  defaults: {
    VRow: { density: 'compact' },
    VCard: { rounded: 'xl', elevation: 0 },
    VChip: { rounded: 'pill' },
    VTextField: {
      variant: 'solo-filled',
    },
    VSelect: {
      variant: 'solo-filled',
    },
    VCombobox: {
      variant: 'solo-filled',
    },
    VTextarea: {
      variant: 'solo-filled',
    },
  },
  theme: {
    // obhod: как в приложении — по умолчанию тёмная тема, бирюзовый акцент
    defaultTheme: localStorage.getItem('theme') ?? 'dark',
    themes: {
      light: {
        colors: {
          primary: '#14916f',
          secondary: '#6d5ce0',
          success: '#14916f',
          info: '#2f7fdc',
          warning: '#c98510',
          error: '#d23f3f',
          background: '#e6edf8',
          surface: '#ffffff',
        },
      },
      dark: {
        colors: {
          primary: '#2dd4a7',
          secondary: '#9b8cff',
          success: '#2dd4a7',
          info: '#5aa8ff',
          warning: '#f5b84a',
          error: '#f06b6b',
          background: '#070b16',
          surface: '#141b2a',
          'on-primary': '#04211a',
        },
      },
    },
  },
  locale: {
    locale: localStorage.getItem("locale") ?? 'en',
    fallback: 'en',
    messages: { en, fa, vi, zhHans, zhHant, ru },
  },
})
