# Wie acme-dns funktioniert

Beispiel in diesem Dokument: die Domain **`beispiel.de`** soll ein Let's-Encrypt-Zertifikat
bekommen, ausgestellt über Traefik.

## Das Prinzip in einem Satz

Let's Encrypt verlangt für ein Zertifikat einen TXT-Eintrag unter
`_acme-challenge.beispiel.de` — statt dafür einen API-Zugang zur echten Zone von
`beispiel.de` zu brauchen, zeigt dort **einmalig** ein CNAME auf unseren acme-dns-Server,
der den wechselnden TXT-Wert dann selbst ausliefert.

## Wo welcher Eintrag steht

```mermaid
flowchart TB
    subgraph kunde["① Zone beispiel.de · beim DNS-Anbieter der Domain"]
        CNAME["_acme-challenge.beispiel.de<br/><b>CNAME</b><br/>a1b2c3d4-....acme-dns.netzint.de"]
    end

    subgraph nz["② Zone netzint.de · einmalig eingerichtet, steht bereits"]
        A["acme-dns.netzint.de<br/><b>A</b> 185.50.122.60"]
        NS["acme-dns.netzint.de<br/><b>NS</b> acme-dns.netzint.de"]
    end

    subgraph srv["③ acme-dns Server · 185.50.122.60"]
        TXT["a1b2c3d4-....acme-dns.netzint.de<br/><b>TXT</b> wechselnder Challenge-Wert"]
        DB[("SQLite")]
        TXT --- DB
    end

    CNAME ==>|"zeigt auf"| NS
    NS ==>|"delegiert alles darunter an"| TXT

    style CNAME fill:#1e3a5f,stroke:#38bdf8,color:#e6edf5
    style TXT fill:#1e3a5f,stroke:#38bdf8,color:#e6edf5
    style A fill:#16233a,stroke:#2f4157,color:#e6edf5
    style NS fill:#16233a,stroke:#2f4157,color:#e6edf5
    style DB fill:#16233a,stroke:#2f4157,color:#e6edf5
```

① muss **einmal von Hand** angelegt werden — danach nie wieder.
② steht bereits und gilt für alle Domains gemeinsam.
③ setzt acme-dns selbst, per API vom ACME-Client. Nie von Hand.

### Die Einträge im Klartext

| Wo | Eintrag | Wann |
| --- | --- | --- |
| Zone `netzint.de` | `acme-dns.netzint.de.  A   185.50.122.60` | einmalig, steht bereits |
| Zone `netzint.de` | `acme-dns.netzint.de.  NS  acme-dns.netzint.de.` | einmalig, steht bereits |
| Zone `beispiel.de` | `_acme-challenge.beispiel.de.  CNAME  a1b2c3d4-....acme-dns.netzint.de.` | **einmal pro Domain, von Hand** |
| — | der TXT-Wert | nie von Hand — den setzt der ACME-Client automatisch |

Der `NS`-Eintrag ist der eigentliche Trick: er sagt dem Internet „für alles unterhalb von
`acme-dns.netzint.de` ist dieser Server zuständig". Deshalb kann acme-dns dort beliebige
TXT-Werte ausliefern, ohne dass jemand eine Zonendatei bearbeitet.

## Wie eine Ausstellung abläuft

```mermaid
sequenceDiagram
    autonumber
    participant T as Traefik (lego)
    participant LE as Let's Encrypt
    participant AD as acme-dns
    participant DNS as öffentliche Resolver

    T->>LE: Zertifikat für beispiel.de bitte
    LE-->>T: Challenge-Token (43 Zeichen)

    Note over T: liest Zugangsdaten aus<br/>acme-dns.json

    T->>AD: POST /update<br/>X-Api-User + X-Api-Key<br/>{subdomain, txt: Token}
    AD->>AD: Token als TXT für<br/>a1b2c3d4-....acme-dns.netzint.de speichern
    AD-->>T: ok

    T->>LE: bin fertig, bitte prüfen

    LE->>DNS: TXT für _acme-challenge.beispiel.de?
    DNS->>DNS: CNAME gefunden,<br/>folge nach acme-dns.netzint.de
    DNS->>AD: TXT für a1b2c3d4-...?
    AD-->>DNS: der gespeicherte Token
    DNS-->>LE: Token

    LE->>LE: stimmt mit dem ausgegebenen überein
    LE-->>T: Zertifikat

    Note over T,LE: Bei jeder Verlängerung wieder ab Schritt 1 —<br/>der CNAME bleibt dabei unverändert.
```

## Was die Oberfläche dabei macht

```mermaid
flowchart TB
    S1["<b>1 · Domain anlegen</b><br/>erzeugt Subdomain und Zugangsdaten"]
    S2["<b>2 · Daten übernehmen</b><br/>CNAME in die Zone, Zugangsdaten in Traefik"]
    S3["<b>3 · Prüfen</b><br/>Knopf in der Oberfläche"]
    C1["prüft den CNAME über öffentliche Resolver"]
    C2["schreibt einen Zufallswert und wartet,<br/>bis er wirklich auflösbar ist"]
    OK["fertig — ab jetzt läuft alles automatisch"]

    S1 --> S2 --> S3
    S3 --> C1 --> OK
    S3 --> C2 --> OK

    style S1 fill:#16233a,stroke:#2f4157,color:#e6edf5
    style S2 fill:#16233a,stroke:#2f4157,color:#e6edf5
    style S3 fill:#1e3a5f,stroke:#38bdf8,color:#e6edf5
    style C1 fill:#101822,stroke:#2f4157,color:#8da2b8
    style C2 fill:#101822,stroke:#2f4157,color:#8da2b8
    style OK fill:#173a2a,stroke:#4ade80,color:#e6edf5
```

Schritt 3 prüft damit nicht nur, ob der CNAME formal richtig aussieht, sondern ob der
komplette Weg bis zum TXT-Wert wirklich funktioniert. Eine kaputte Delegierung oder ein
Tippfehler im Ziel fällt dadurch sofort auf, statt erst beim nächsten Ausstellversuch.

## Warum der Umweg überhaupt

Die naheliegende Alternative wäre, dem ACME-Client einen API-Schlüssel für die Zone von
`beispiel.de` zu geben. Der könnte dann aber **jeden** Eintrag der Domain ändern — MX,
A-Records, alles. Ein kompromittierter Webserver hätte damit die Kontrolle über die
gesamte Domain.

Mit acme-dns bekommt jeder Dienst stattdessen Zugangsdaten, die ausschließlich einen TXT-Wert
unter **einer einzigen** UUID-Subdomain setzen können. Mehr Schaden ist damit nicht möglich.

```mermaid
flowchart LR
    subgraph ohne["ohne acme-dns"]
        W1["Webserver"] -->|"API-Schlüssel"| Z1["ganze Zone beispiel.de<br/>MX, A, alles"]
    end

    subgraph mit["mit acme-dns"]
        W2["Webserver"] -->|"Zugangsdaten"| Z2["genau ein TXT-Eintrag<br/>unter einer UUID"]
    end

    style Z1 fill:#3a1e1e,stroke:#f87171,color:#e6edf5
    style Z2 fill:#173a2a,stroke:#4ade80,color:#e6edf5
    style W1 fill:#16233a,stroke:#2f4157,color:#e6edf5
    style W2 fill:#16233a,stroke:#2f4157,color:#e6edf5
```

## Wenn etwas nicht geht

| Symptom | Ursache |
| --- | --- |
| Prüfung meldet „kein CNAME-Eintrag" | Der CNAME in der Zone von `beispiel.de` fehlt oder ist noch nicht propagiert |
| Prüfung meldet „zeigt auf das falsche Ziel" | CNAME zeigt auf eine andere Registrierung — Ziel aus dem Detail-Dialog vergleichen |
| CNAME stimmt, TXT-Test schlägt fehl | Delegierung erreicht den Server nicht, oder ein Resolver hält noch einen alten Eintrag |
| Client meldet 401 beim `/update` | Passwort passt nicht mehr — im Detail-Dialog „Neue Zugangsdaten" erzeugen und in Traefik nachtragen |

Wichtig bei allen Fehlern: **der CNAME bleibt gültig**. Selbst wenn Zugangsdaten neu
erzeugt werden, ändert sich die Subdomain nicht — in der Zone von `beispiel.de` muss also
nie etwas nachgezogen werden.
