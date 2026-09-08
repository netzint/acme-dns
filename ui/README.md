# acme-dns Verwaltungsoberfläche

React 19 + Vite + Tailwind CSS v4. Die Komponenten kommen aus
[shadcn/ui](https://ui.shadcn.com) und [ReUI](https://reui.io) und liegen als Quelltext im
Repository, nicht als npm-Abhängigkeit.

## Entwicklung

```bash
npm install
npm run dev
```

Der Dev-Server proxyt `/api`, `/health`, `/register` und `/update` auf
`http://127.0.0.1:8080`. Ein lokal laufendes acme-dns auf einem anderen Port erreichst du
über `ACMEDNS_BACKEND=http://127.0.0.1:8087 npm run dev`.

```bash
npm run build   # tsc -b && vite build, Ergebnis in dist/
npm run lint    # oxlint
```

Im Produktionsbetrieb liefert das Go-Binary den Inhalt von `dist/` selbst aus, die API
liegt also auf derselben Origin. Für den Betrieb ohne Docker zeigt `api.ui_path` in der
`config.cfg` auf dieses Verzeichnis.

## Komponenten aktualisieren oder ergänzen

Die ReUI-Registry ist in `components.json` als Namensraum `@reui` hinterlegt:

```bash
npx shadcn@latest add @reui/stepper --overwrite   # vorhandene aktualisieren
npx shadcn@latest add @reui/timeline              # neue hinzufügen
npx shadcn@latest add card dialog                 # aus der shadcn-Basis
```

Alles unter `src/components/ui` und `src/components/reui` ist so erzeugter Fremdcode.
Diese beiden Verzeichnisse sind vom Linter ausgenommen und die `noUnused*`-Prüfungen von
TypeScript sind projektweit aus, damit ein erneutes Ausführen der CLI nicht jedes Mal
Nacharbeit erzeugt. Eigene Bausteine gehören nach `src/components` (eine Ebene darüber),
`src/features` oder `src/pages`.

## Aufbau

| Pfad | Inhalt |
| --- | --- |
| `src/lib/api.ts` | typisierter Client für `/api/admin/*`, Sitzungsverwaltung |
| `src/lib/snippets.ts` | erzeugt die Copy-und-Paste-Blöcke für Traefik, lego, certbot, curl |
| `src/lib/types.ts` | die Datenstrukturen der API |
| `src/pages/` | Login und Domainübersicht |
| `src/features/` | die Dialoge: anlegen, Details, DNS-Prüfung, Bestätigung |
