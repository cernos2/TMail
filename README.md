# TMail

**English** · [Русский](README.ru.md)

A complete, self-hosted mail server with a modern webmail and an AI assistant — installed on one
Linux server with one command.

* **Mail server**: Postfix (SMTP), Dovecot (IMAP/POP3, quotas, server-side filters), Rspamd (spam,
  DKIM signing, SPF/DKIM/DMARC checks), ClamAV (antivirus: attachments cannot be downloaded until
  they are checked), Let's Encrypt certificates, PostgreSQL as the single source of truth.
* **Webmail**: Gmail-like interface in 100 languages, conversations, fast full-text search (also
  inside attachments), previews of PDF/Office/images, large files as links (Garage S3), labels,
  folders with auto-sorting, calendar with reminders, templates, signatures with images, delayed
  send with undo, spell checking, browser notifications, dark mode, phone layout.
* **Several mailboxes per user**: one login works with many mailboxes; replies go from the address
  the letter came to. Collector for external mailboxes (Gmail, any IMAP/POP3) with sending through
  their own SMTP.
* **AI assistant** (optional, any OpenAI-compatible API — OpenAI, OpenRouter, or your own model on
  vLLM/Ollama): replies, summaries, rewriting, dictation, auto-replies, sorting, and contact cards
  of companies and people extracted from letters and their attachments. Prompts are editable in the
  admin panel. MCP access for external AI agents.
* **Administration**: users and roles, mailboxes, aliases, domains with live DNS checks, collector,
  antivirus, server health, audit log. Watchdog, fail2ban, firewall and encrypted incremental
  backups are set up by the installer.

## See it

<p align="center"><img src="docs/screenshots/en/02-inbox-all-mailboxes.webp" alt="TMail inbox: one login, several mailboxes"></p>

▶️ **[Watch the 2-minute tour (MP4)](docs/video/tmail-tour.mp4)** · all screenshots, in English and Russian: [docs/screenshots](docs/screenshots/LIST.md)

| New mail arrives live | Reply with AI |
|---|---|
| <img src="docs/video/new-mail.webp" alt="New mail arriving"> | <img src="docs/video/reply-with-ai.webp" alt="Reply with AI"> |
| **Send with Undo** | **Spell check that fixes misplaced spaces** |
| <img src="docs/video/send-undo.webp" alt="Send with undo"> | <img src="docs/video/spell-check.webp" alt="Spell check"> |

| | |
|---|---|
| <img src="docs/screenshots/en/04-conversation-counterparty.webp" alt="Who wrote and who answered: company, tax ID and phones of the sender"><br>**Who wrote and who answered: company, tax ID and phones of the sender** | <img src="docs/screenshots/en/16-reply-from-shared-address.webp" alt="The reply goes from the mailbox the letter came to"><br>**The reply goes from the mailbox the letter came to** |
| <img src="docs/screenshots/en/06-preview-pdf.webp" alt="PDF, Office and image previews built in"><br>**PDF, Office and image previews built in** | <img src="docs/screenshots/en/09-preview-scan-ocr.webp" alt="Scanned documents: select and copy the recognised text"><br>**Scanned documents: select and copy the recognised text** |
| <img src="docs/screenshots/en/11-attachments-antivirus-zip.webp" alt="Every attachment checked by the antivirus; download all as zip"><br>**Every attachment checked by the antivirus; download all as zip** | <img src="docs/screenshots/en/14-phishing-warning.webp" alt="Phishing warning: failed SPF/DKIM/DMARC, risky links marked"><br>**Phishing warning: failed SPF/DKIM/DMARC, risky links marked** |
| <img src="docs/screenshots/en/22-context-menu.webp" alt="Right-click: formatting, dictation, AI rephrase and fix"><br>**Right-click: formatting, dictation, AI rephrase and fix** | <img src="docs/screenshots/en/17-compose-templates.webp" alt="Reply templates, with AI picking the right one"><br>**Reply templates, with AI picking the right one** |
| <img src="docs/screenshots/en/24-ai-reply-questions.webp" alt="AI finds the questions of a letter and offers answers"><br>**AI finds the questions of a letter and offers answers** | <img src="docs/screenshots/en/23-ai-summary.webp" alt="AI summary of a long letter"><br>**AI summary of a long letter** |
| <img src="docs/screenshots/en/30-calendar-reminder.webp" alt="Calendar, reminders, goals, tasks and notes next to the mail"><br>**Calendar, reminders, goals, tasks and notes next to the mail** | <img src="docs/screenshots/en/31-contact-company.webp" alt="Contact cards built from the correspondence, for any country"><br>**Contact cards built from the correspondence, for any country** |
| <img src="docs/screenshots/en/27-search-filters.webp" alt="Fast search across all mailboxes, with filters"><br>**Fast search across all mailboxes, with filters** | <img src="docs/screenshots/en/33-settings-appearance.webp" alt="Colour schemes, backgrounds, dark mode"><br>**Colour schemes, backgrounds, dark mode** |
| <img src="docs/screenshots/en/40-settings-profile-languages.webp" alt="100 interface languages, per user, with time zone"><br>**100 interface languages, per user, with time zone** | <img src="docs/screenshots/en/38-settings-mail-apps.webp" alt="Ready settings for phones and desktop mail apps"><br>**Ready settings for phones and desktop mail apps** |
| <img src="docs/screenshots/en/41-admin-overview.webp" alt="Admin overview: what needs attention, service health"><br>**Admin overview: what needs attention, service health** | <img src="docs/screenshots/en/43-admin-user-editor.webp" alt="Users, roles and access to shared mailboxes"><br>**Users, roles and access to shared mailboxes** |
| <img src="docs/screenshots/en/46-admin-domains-dns.webp" alt="Domains with live DNS checks (MX, SPF, DKIM, DMARC, PTR)"><br>**Domains with live DNS checks (MX, SPF, DKIM, DMARC, PTR)** | <img src="docs/screenshots/en/47-admin-ai-assistant.webp" alt="Any AI: OpenAI, OpenRouter or your own model"><br>**Any AI: OpenAI, OpenRouter or your own model** |
| <img src="docs/screenshots/en/50-admin-antivirus.webp" alt="Antivirus with quarantine"><br>**Antivirus with quarantine** | <img src="docs/screenshots/en/51-admin-server.webp" alt="Server health: CPU, memory, disk, queue, certificates"><br>**Server health: CPU, memory, disk, queue, certificates** |
| <img src="docs/screenshots/en/61-dark-reading.webp" alt="Dark theme"><br>**Dark theme** | <img src="docs/screenshots/en/70-phone-inbox.webp" alt="Works on phones"><br>**Works on phones** |

Try the interface on your computer with invented data (no mail server needed):
`cd web && npm ci && npx nuxt generate && node demo/server.mjs --lang en` → http://127.0.0.1:8790

## Built for teams that share mailboxes

Most companies have mailboxes that several people answer: `sales@`, `support@`, `info@`, `orders@`.
TMail is designed around that:

* **One login, many mailboxes.** Each employee signs in once and sees every mailbox they were given
  access to — together in one list or one by one. Access is granted per mailbox, read-only or with
  sending, and taken away in one click; nobody shares a mailbox password.
* **Replies go from the right address.** A reply is sent from the mailbox the letter came to, with
  that mailbox's signature.
* **Everyone sees who answered.** Sent letters show which employee sent them; the conversation, read
  state, labels and folders are shared by the team in real time.
* **External mailboxes too.** Gmail or any IMAP/POP3 mailbox can be connected and used by the team
  the same way, sending through its own SMTP.
* Plus contact cards of companies and people built from the correspondence, so the whole team knows
  who a sender is, their company, tax ID and phones.

## Requirements — light by design

TMail is written to run on small servers. Every part is chosen and tuned for memory: the webmail
backend is a single Go binary (~40 MB of RAM), search uses a PostgreSQL index instead of a separate
search server, and the antivirus is configured not to double its memory while updating.

| | Minimum | Recommended |
|---|---|---|
| Memory | **2 GB** (+ swap, created by the installer) | 4 GB |
| CPU | 1 core | 2+ cores |
| Disk | 20 GB + mail | SSD |

Measured on a production server (4 vCPU, 6 GB RAM) holding **66 GB of mail, 146,000 indexed
letters**, with the antivirus, spam filter, document previews and AI features on: all services
together keep **about 1.2 GB in memory** (plus about 1.4 GB of rarely used data — mostly the
antivirus signatures — in swap).

How that compares with the minimums the other self-hosted mail servers publish (with antivirus):

| | Minimum memory |
|---|---|
| **TMail** | **2 GB** |
| Mailu | 3 GB + 1 GB swap ([docs](https://mailu.io/2024.06/compose/requirements.html)) |
| iRedMail | 4 GB recommended ([forum](https://forum.iredmail.org/post88548.html)) |
| mailcow | 6 GB + 1 GB swap ([docs](https://docs.mailcow.email/getstarted/prerequisite-system/)) |

Other requirements:

* A public IPv4 address; Ubuntu 22.04+ or Debian 12+ (other Linux distributions with Docker work too).
* **Outgoing port 25** must be open (some hosting providers block it until asked).
* A domain whose DNS records you can edit, and reverse DNS (PTR) for the server's IP address
  (set at the hosting provider).

## Installation

```sh
git clone https://github.com/cernos2/TMail.git /opt/tmail
cd /opt/tmail
sudo ./install.sh
```

The installer checks the server, installs Docker, asks for the domain, the administrator, the
interface language, the country (company details and phone numbers are recognised for it), the time
zone, the AI service and where to keep backups; then it builds and starts everything, creates the
domain with its DKIM key, the first mailbox and the administrator, and prints the DNS records to add.
Everything can be passed as options for an unattended installation — see `./install.sh --help`.

The credentials are saved to `/root/tmail-credentials.txt`. **Keep the backup password somewhere
outside the server**: backups cannot be read without it.

### DNS records

| Name | Type | Value |
|---|---|---|
| `mail.example.com` | A | server IP |
| `files.example.com` | A | server IP |
| `example.com` | MX | `10 mail.example.com` |
| `example.com` | TXT | `v=spf1 mx -all` |
| `<selector>._domainkey.example.com` | TXT | printed by the installer (`docker compose run --rm ctl dns example.com`) |
| `_dmarc.example.com` | TXT | `v=DMARC1; p=quarantine; rua=mailto:postmaster@example.com` |
| PTR of the server IP | | `mail.example.com` (at the hosting provider) |

The admin panel (**Domains**) checks every record live. The HTTPS certificate is issued
automatically as soon as `mail.example.com` points to the server.

## AI assistant

Choose the service in **Admin → AI assistant**: OpenAI, OpenRouter (hundreds of models with one key),
or your own model behind any OpenAI-compatible API. The assistant's name, the default model, a
separate speech-recognition API (OpenRouter has none) and every prompt are configurable there. To
read scanned documents and PDF pages the model must accept images.

## Everyday operation

```sh
cd /opt/tmail
docker compose ps                          # state of every service
docker compose logs -f postfix dovecot     # logs
alias tmail='docker compose run --rm ctl'
tmail mailbox add anna@example.com --name "Anna Smith"
tmail user add anna --name "Anna Smith" --mailboxes anna@example.com,info@example.com
tmail domain add example.org && tmail dkim gen example.org && tmail dns example.org
```

Mail programs: IMAP `mail.example.com:993` (SSL/TLS) or POP3 `:995`; SMTP `:465` (SSL/TLS) or
`:587` (STARTTLS); the login is the full address. Each user finds ready settings (and an iPhone/Mac
profile) in the webmail settings.

### Update

```sh
cd /opt/tmail && git pull && sudo ./install.sh --yes
```

The installer keeps the settings and the data; it rebuilds the services, applies database changes
and rebuilds the web interface.

### Backups

`deploy/backup.sh` runs every night (restic: incremental, deduplicated, encrypted; 7 daily, 4 weekly,
3 monthly snapshots). Set `BACKUP_REPOSITORY` in `.env` to S3 (`s3:https://host/bucket` with
`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`) or SFTP (`sftp:user@host:/path`) to keep them off the
server. Restore — on the same or a new server:

```sh
BACKUP_REPOSITORY=… BACKUP_PASSWORD=… deploy/restore.sh --list
BACKUP_REPOSITORY=… BACKUP_PASSWORD=… deploy/restore.sh latest /opt/tmail
```

## License

TMail is free software under the [GNU Affero General Public License v3.0](LICENSE) with an
additional attribution term (see [NOTICE](NOTICE)): if you run TMail — changed or not — for other
people, keep the "TMail" link to this repository visible in the interface, and publish your changes
under the same license.

## Translations

The interface texts are in `web/app/locales/<language>.json`; see [web/I18N.md](web/I18N.md).
Corrections by native speakers are very welcome.

## Layout

| Path | What |
|---|---|
| `install.sh` | installer and updater |
| `compose.yml` | every service (host network; internal ports only on 127.0.0.1) |
| `services/` | Postfix, Dovecot, Rspamd, Unbound, ClamAV, certificates, Garage, document previews |
| `backend/` | `zmaild` (Go): webmail API, IMAP/SMTP on behalf of mailboxes, live updates, admin, AI |
| `ctl/` | `zmail` command-line tool and the database schema (`ctl/migrations`) |
| `web/` | the web interface (Nuxt 4) |
| `deploy/` | backups, restore, firewall, fail2ban, watchdog, web build |
