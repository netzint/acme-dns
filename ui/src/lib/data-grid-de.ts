import type { DataGridI18nOverrides } from '@/components/reui/data-grid/data-grid-i18n'

/** German labels for the ReUI data grid; the defaults ship in English. */
export const dataGridDe: DataGridI18nOverrides = {
  labels: {
    sortAscending: 'Aufsteigend',
    sortDescending: 'Absteigend',
    columnsMenu: 'Spalten',
    toggleColumns: 'Spalten ein- und ausblenden',
    loading: 'Wird geladen …',
    empty: 'Keine Einträge',
    rowsPerPage: 'Zeilen pro Seite',
    paginationInfo: ({ from, to, count }) => `${from}–${to} von ${count}`,
    previousPage: 'Vorherige Seite',
    nextPage: 'Nächste Seite',
    goToPage: (page) => `Zu Seite ${page}`,
    filterSelectedCount: (count) => `${count} ausgewählt`,
    filterNoResults: 'Keine Treffer',
    filterClear: 'Filter zurücksetzen',
  },
}
