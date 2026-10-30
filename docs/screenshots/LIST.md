# Screenshots and videos

All material comes from the demo server (`web/demo/server.mjs`) with invented data — a fictional company,
Nordwave Engineering & Trading, on `nordwave.example`. The same file names exist in `en/` (English
interface and data) and `ru/` (Russian interface and data). Desktop shots are 1440×900; phone shots are 390×844 at 2×.

| File | What it shows |
|---|---|
| 01-sign-in.webp | Sign-in page |
| 02-inbox-all-mailboxes.webp | One login, several mailboxes: the "All mailboxes" inbox with the personal, shared (sales@, support@, info@) and collected Gmail mailboxes, coloured mailbox markers, unread counts, labels, attachment chips, and the calendar beside it |
| 03-shared-mailbox.webp | Switching to one shared mailbox (sales@) with its own folders; a conversation open in it |
| 04-conversation-counterparty.webp | A letter sent from the shared mailbox: "Sent by: mark (Mark Ellison)", the counterparty box (company, tax ID, register number, phones, e-mail, copy buttons), and the earlier letter folded above |
| 05-thread.webp | A five-letter conversation (Gmail-style threading, our replies from Sent included) with labels and the counterparty box |
| 06-preview-pdf.webp | Built-in PDF preview of a purchase order, with "Copy all text" and Download |
| 07-preview-xlsx.webp | Spreadsheet preview of an XLSX price list, with sheet tabs |
| 08-preview-docx.webp | DOCX contract shown as a document preview |
| 09-preview-scan-ocr.webp | Scanned delivery note (an image-only PDF): the recognised (OCR) text can be selected and copied |
| 10-preview-photo.webp | Photo attachment in the full-screen viewer |
| 11-attachments-antivirus-zip.webp | Attachment cards with thumbnails, the antivirus "checked" badge and "Download all (zip)" |
| 12-large-file-link.webp | Large files sent as download links (the "Files by link" card in the letter) |
| 13-large-file-page.webp | The download page of a large file (preview, size, expiry date, "Checked by antivirus") |
| 14-phishing-warning.webp | A phishing letter: SPF/DKIM/DMARC failed, red warning, risky links marked with ⚠ |
| 15-newsletter-images-blocked.webp | Newsletter with remote images blocked ("Show images" / "Always show from…") |
| 16-reply-from-shared-address.webp | Inline reply: the "the message came to sales@…, so the reply goes from the same address" hint, the mailbox signature and the quoted letter |
| 17-compose-templates.webp | The "Templates" menu in the composer, with "Suggest from the message" (AI picks a template) |
| 18-compose-loggi-menu.webp | The Loggi (AI assistant) menu in the composer: write a reply or improve the text from short instructions |
| 19-send-undo.webp | Delayed sending: the "Sending in 14 s · Undo" toast |
| 20-forgotten-attachment.webp | "Did you forget to attach a file?" warning on send |
| 21-spellcheck-tooltip.webp | Spell check: a misplaced space ("thisf eature" / «этотф ункционал») underlined, the fix offered in a tooltip |
| 22-context-menu.webp | Right-click menu in the editor: spelling fix, formatting, colours, fonts, clipboard, dictation and AI actions (dictate, rephrase, fix mistakes) |
| 23-ai-summary.webp | AI summary of a long letter (who, what they ask, what to do) |
| 24-ai-reply-questions.webp | "Reply with AI": Loggi lists the questions of the letter with one-click answers and things to ask the sender |
| 25-ai-written-reply.webp | The reply Loggi wrote from those answers, in the composer with the signature |
| 26-search.webp | Search across all mailboxes with results |
| 27-search-filters.webp | Search filters: "Unread" on, the period popover open (today, 7 days, this month, custom dates), "Attachments" |
| 28-new-mail-arrives.webp | New mail arriving live: highlighted row at the top and the "New message" toast |
| 29-new-in-conversation.webp | "New message in this conversation from …" banner over the open thread |
| 30-calendar-reminder.webp | Calendar month with events, goals, tasks and notes for the month, and a reminder popup (Done / in 15 min / in 1 hour) |
| 31-contact-company.webp | Contacts (counterparty cards): list and a company card with country, tax IDs, bank details, contacts and links (our company as buyer) |
| 32-contact-links.webp | A customer's card: supplier link, company details, delivery address, contact persons |
| 33-settings-appearance.webp | Settings → Appearance: unread style, backgrounds, image policy, light/dark theme, colour schemes |
| 34-settings-signatures.webp | Settings → Signatures: default sender, rich signature with logo per mailbox, opening text |
| 35-settings-notifications.webp | Settings → Notifications: new mail, auto-sorted mail, calendar reminders |
| 36-settings-templates.webp | Settings → Reply templates: shared and personal templates |
| 37-settings-auto-reply.webp | Settings → Auto-reply: reason, dates, substitute, text written by Loggi |
| 38-settings-mail-apps.webp | Settings → Mail apps: IMAP/POP3/SMTP settings per mailbox, "For iPhone/Mac" profile, how-to for iPhone, Android, Outlook, Thunderbird |
| 39-settings-ai-agents-mcp.webp | Settings → AI agents (MCP): access tokens with limits for Claude and other agents |
| 40-settings-profile-languages.webp | Settings → Profile: photo, interface language picker open (100 languages), time zone |
| 41-admin-overview.webp | Admin → Overview: what needs attention (a DNS warning), counters, service health, recent events |
| 42-admin-users.webp | Admin → Users: roles, mailbox access, last seen, status |
| 43-admin-user-editor.webp | Admin → user editor: name, role, interface language and time zone |
| 53-admin-user-mailbox-access.webp | Admin → user editor, "Mailbox access" tab: which mailboxes, can send, personal |
| 44-admin-mailboxes.webp | Admin → Mailboxes: usage meters, letters, users, external (collected) mailboxes |
| 45-admin-aliases.webp | Admin → Aliases: forwarding addresses |
| 46-admin-domains-dns.webp | Admin → Domains and DNS: live check of MX, SPF, DKIM, DMARC (missing on the second domain), A, PTR, with values to copy |
| 47-admin-ai-assistant.webp | Admin → AI assistant: provider (OpenRouter), model, speech recognition, status and available models |
| 48-admin-mail-collector.webp | Admin → Mail collector: a Gmail-like mailbox synced two-way, an old POP3 server with a sign-in error |
| 49-admin-block-allow-lists.webp | Admin → Blocked and trusted senders |
| 50-admin-antivirus.webp | Admin → Antivirus: ClamAV status, quarantine with an EICAR test file, letters rejected on arrival |
| 51-admin-server.webp | Admin → Server: services, CPU, memory, swap, disk, antivirus, mail queue, spam statistics, certificate |
| 52-admin-audit-log.webp | Admin → Audit log: sign-ins incl. failed attempts, with IP and device |
| 60-dark-inbox.webp | Dark theme: inbox with calendar |
| 61-dark-reading.webp | Dark theme: reading a conversation |
| 62-background-image.webp | Light theme with a background image ("Mountains") behind the side columns |
| 70-phone-inbox.webp | Phone: inbox |
| 71-phone-reading.webp | Phone: reading a conversation with the counterparty box |
| 72-phone-compose.webp | Phone: writing a letter |
| 73-phone-menu.webp | Phone: side menu with folders, mailboxes, settings and administration |

## Video (`docs/video/`)

| File | What it shows |
|---|---|
| tmail-tour.mp4 | 2-minute tour (1280×800, H.264): sign in → inbox across shared mailboxes → conversation with counterparty box → PDF preview → reply with AI (questions → written reply) → templates → send with Undo → new mail arrives → calendar reminder → contact card → admin overview, DNS, antivirus → switching the interface to Russian |
| new-mail.webp | Loop: a new letter arrives (row appears at the top, toast) |
| spell-check.webp | Loop: a misplaced space is underlined and fixed from the tooltip |
| reply-with-ai.webp | Loop: "Reply with AI" — questions, one-click answers, the reply is written |
| send-undo.webp | Loop: send, the "Sending in … · Undo" toast, Undo brings the letter back |
