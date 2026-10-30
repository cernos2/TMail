// Everything around the letters: calendar, planner, labels, folder rules, templates, auto-replies,
// counterparty cards and the administrator's data (domains, aliases, collector, antivirus, audit, AI…).

export function extras(W, now) {
  const { L, D, D2, org, U, P, C, gmail } = W
  const A = 'anna@' + D
  const iso = d => d.toISOString()
  const at = (days, h = 9, m = 0) => { const d = new Date(now); d.setDate(d.getDate() + days); d.setHours(h, m, 0, 0); return d }
  const dayOnly = days => { const d = new Date(now); d.setDate(d.getDate() + days); d.setHours(0, 0, 0, 0); return d }
  const back = (min) => new Date(now.getTime() - min * 60000)

  // ---------- labels and own folders ----------
  const labels = [
    { id: 1, name: L('Urgent', 'Срочно'), color: 'red' },
    { id: 2, name: L('Tender', 'Тендер'), color: 'blue' },
    { id: 3, name: L('Quality', 'Качество'), color: 'purple' },
    { id: 4, name: L('Follow up', 'Вернуться'), color: 'yellow' },
    { id: 5, name: L('Logistics', 'Логистика'), color: 'teal' },
  ]
  const ownFolders = {
    [A]: [L('Suppliers', 'Поставщики'), L('Tidewater project', 'Проект верфи')],
    ['sales@' + D]: [L('Orders', 'Заказы')],
    ['support@' + D]: [L('Warranty', 'Гарантия')],
    ['info@' + D]: [L('Invoices', 'Счета'), L('Newsletters', 'Рассылки')],
    [gmail]: [L('Travel', 'Поездки')],
  }
  const folderRules = [
    { account: A, folder: L('Suppliers', 'Поставщики'), description: L('Letters from our suppliers: castings, seals, bearings, actuators. Price lists, stock and delivery questions.', 'Письма поставщиков: отливки, уплотнения, подшипники, приводы. Прайс-листы, наличие, сроки поставки.'),
      senders: ['@brightline-parts.example', '@hengda-valve.example', 'm.huber@alpenwerk.example'], auto: true, hideInbox: false, expireDays: 0 },
    { account: A, folder: L('Tidewater project', 'Проект верфи'), description: L('Everything about the Tidewater tug pumps tender.', 'Всё по тендеру на насосы для буксиров.'), senders: [], auto: false, hideInbox: false, expireDays: 0 },
    { account: 'info@' + D, folder: L('Invoices', 'Счета'), description: L('Invoices, payment reminders and statements from suppliers and service providers.', 'Счета, напоминания об оплате и акты сверки от поставщиков и подрядчиков.'),
      senders: ['invoices@brightline-parts.example', 'buh@brightline-parts.example'], auto: true, hideInbox: true, expireDays: 0 },
    { account: 'info@' + D, folder: L('Newsletters', 'Рассылки'), description: L('Newsletters, webinars and other mass mailings.', 'Новостные рассылки, вебинары и прочие массовые письма.'), senders: ['@pumpweekly.example'], auto: true, hideInbox: true, expireDays: 30 },
  ]

  // ---------- calendar ----------
  let eid = 0
  const ev = (e) => ({ id: ++eid, notes: '', end: null, allDay: false, color: 'blue', isTask: false, done: false, remindMinutes: 15, ...e })
  const events = [
    ev({ title: L('Call with Tidewater — offer Q-2611', 'Звонок с верфью — КП № 2611'), start: iso(at(1, 15, 0)), end: iso(at(1, 15, 30)), color: 'blue', notes: L('Femke de Vries: lead time, discount, 30/70 payment, spare parts kits.', 'Елена Фролова: срок, скидка, оплата 30/70, комплекты ЗИП.') }),
    ev({ title: L('Discount for Tidewater — with Peter and Mark', 'Скидка для верфи — с Петром и Максимом'), start: iso(at(nextDow(now, 2), 10, 0)), end: iso(at(nextDow(now, 2), 11, 0)), color: 'green' }),
    ev({ title: L('Pickup: 1 pallet to Rotterdam (HL-55107)', 'Забор груза: 1 паллета на верфь (ГЛ-55107)'), start: iso(at(nextDow(now, 1), 9, 0)), end: iso(at(nextDow(now, 1), 9, 30)), color: 'teal' }),
    ev({ title: L('Site visit: Tidewater pump P3 alignment', 'Выезд: центровка насоса P3 на верфи'), start: iso(at(nextDow(now, 5), 9, 0)), end: iso(at(nextDow(now, 5), 13, 0)), color: 'orange', notes: L('Sofia, laser alignment kit.', 'София, лазерный центровщик.') }),
    ev({ title: 'Pump Expo 2026', start: iso(dayOnly(12)), end: iso(dayOnly(14)), allDay: true, color: 'purple', remindMinutes: 1440, notes: L('Stand B-214. Sign the Kestrel agreement.', 'Стенд B-214. Подписать договор с «Кестрел».') }),
    ev({ title: L('Send comments on the Kestrel agreement', 'Отправить замечания к договору «Кестрел»'), start: iso(at(5, 12, 0)), isTask: true, color: 'red' }),
    ev({ title: L('Approve Brightline invoice 418', 'Согласовать счёт «Брайтлайн» № 418'), start: iso(at(-1, 11, 0)), isTask: true, done: true, color: 'yellow' }),
    ev({ title: L('Weekly sales meeting', 'Планёрка отдела продаж'), start: iso(at(nextDow(now, 1) - 7, 9, 30)), end: iso(at(nextDow(now, 1) - 7, 10, 15)), color: 'green', remindMinutes: 5 }),
    ev({ title: L('Weekly sales meeting', 'Планёрка отдела продаж'), start: iso(at(nextDow(now, 1), 9, 30)), end: iso(at(nextDow(now, 1), 10, 15)), color: 'green', remindMinutes: 5 }),
    ev({ title: L('Weekly sales meeting', 'Планёрка отдела продаж'), start: iso(at(nextDow(now, 1) + 7, 9, 30)), end: iso(at(nextDow(now, 1) + 7, 10, 15)), color: 'green', remindMinutes: 5 }),
    ev({ title: L('Weekly sales meeting', 'Планёрка отдела продаж'), start: iso(at(nextDow(now, 1) + 14, 9, 30)), end: iso(at(nextDow(now, 1) + 14, 10, 15)), color: 'green', remindMinutes: 5 }),
    ev({ title: L('Q4 pipeline review', 'Разбор воронки на IV квартал'), start: iso(at(-6, 14, 0)), end: iso(at(-6, 15, 30)), color: 'gray' }),
    ev({ title: L('Maple Ridge: actuators delivered', 'Водоканал: приводы доставлены'), start: iso(dayOnly(0)), allDay: true, color: 'teal', remindMinutes: null }),
    ev({ title: L('Delivery to Tidewater — first 10 pumps', 'Поставка на верфь — первые 10 насосов'), start: iso(dayOnly(36)), allDay: true, color: 'orange', remindMinutes: 1440 }),
    ev({ title: L('Year-end stocktaking', 'Годовая инвентаризация'), start: iso(dayOnly(45)), end: iso(dayOnly(46)), allDay: true, color: 'gray', remindMinutes: null }),
    ev({ title: L('Dentist', 'Стоматолог'), start: iso(at(3, 18, 0)), end: iso(at(3, 19, 0)), color: 'gray', remindMinutes: 60 }),
    ev({ title: L('Kestrel agreement — sign-off with Peter', 'Договор «Кестрел» — согласовать с Петром'), start: iso(at(9, 16, 0)), end: iso(at(9, 16, 45)), color: 'yellow' }),
    ev({ title: L('Call Alpenwerk about AW-2291 shipping', 'Позвонить в Alpenwerk по отгрузке AW-2291'), start: iso(at(8, 11, 0)), isTask: true, color: 'blue' }),
  ]
  // The reminder that is due right now: shown as soon as the demo is opened.
  const due = ev({ title: L('Reply to Tidewater before the call', 'Ответить верфи до звонка'), start: iso(new Date(now.getTime() + 25 * 60000)), isTask: true, color: 'red', remindMinutes: 30,
    notes: L('Answer the four questions in Femke\'s letter.', 'Ответить на четыре вопроса из письма Елены.') })
  events.push(due)

  const plan = {
    goals: [
      { text: L('Win the Tidewater tender (40 pumps)', 'Выиграть тендер верфи (40 насосов)'), done: false },
      { text: L('Sign the Kestrel distribution agreement', 'Подписать дилерский договор с «Кестрел»'), done: false },
      { text: L('Parts shop: +15% revenue vs. last month', 'Магазин запчастей: +15% к прошлому месяцу'), done: true },
    ],
    tasks: [
      { text: L('Cost calculation for spare parts kits', 'Расчёт себестоимости комплектов ЗИП'), done: true },
      { text: L('Prepare the Pump Expo stand B-214', 'Подготовить стенд B-214 на выставку'), done: false },
      { text: L('New price list for 2027', 'Новый прайс-лист на 2027 год'), done: false },
      { text: L('Check Hengda re-cast HV-1187', 'Проверить переливку HV-1187 от Hengda'), done: false },
    ],
    notes: L('Tidewater: max 4% discount (6% with 50% upfront). Spare parts kits at cost.\nKestrel: ask for 25% discount and 45 days payment.', 'Верфь: скидка до 4% (6% при 50% предоплаты). ЗИП — по себестоимости.\n«Кестрел»: просить скидку 25% и отсрочку 45 дней.'),
  }

  // ---------- templates, auto-replies ----------
  const templates = [
    { id: 1, title: L('Thanks for your enquiry', 'Спасибо за запрос'), body: L('Thank you for your enquiry. We are preparing an offer and will send it within one working day.\n\nIf you have drawings or a specification, please send them — it helps us choose the right pump.', 'Спасибо за запрос! Готовим предложение и пришлём его в течение рабочего дня.\n\nЕсли есть чертежи или техническое задание — пришлите, это поможет точнее подобрать насос.'), mailbox: '', shared: true, mine: false },
    { id: 2, title: L('Order confirmation', 'Подтверждение заказа'), body: L('Thank you for your order. We confirm it in full; the goods will be shipped on {date}. You will receive the tracking details from our forwarder.', 'Спасибо за заказ! Подтверждаем его полностью, отгрузка {date}. Данные для отслеживания пришлёт перевозчик.'), mailbox: 'sales@' + D, shared: true, mine: false },
    { id: 3, title: L('Service ticket received', 'Заявка в сервис принята'), body: L('Your request has been registered. A service engineer will contact you within 2 hours. For emergencies call our 24/7 hotline +44 117 555 0199.', 'Ваша заявка зарегистрирована. Сервисный инженер свяжется с вами в течение 2 часов. В экстренных случаях звоните круглосуточно: +7 (812) 555-01-99.'), mailbox: 'support@' + D, shared: true, mine: false },
    { id: 4, title: L('Payment reminder (polite)', 'Напоминание об оплате (вежливо)'), body: L('May I kindly remind you that our invoice is due on {date}. If it has already been paid, please ignore this message.', 'Позвольте напомнить, что срок оплаты нашего счёта — {date}. Если счёт уже оплачен, просто не обращайте внимания на это письмо.'), mailbox: '', shared: false, mine: true },
  ]
  const autoreply = {
    [A]: { mailbox: A, enabled: false, reason: 'trip', dateFrom: ymd(dayOnly(12)), dateTo: ymd(dayOnly(14)), substitute: L('Mark Ellison, mark@nordwave.example', 'Максим Орлов, maxim@nordwave.example'), notes: L('At Pump Expo, stand B-214', 'На выставке «Насосы-Экспо», стенд B-214'),
      subject: L('Out of office until ', 'Не в офисе до ') + fmtDay(dayOnly(14), W.ru),
      body: L(`Thank you for your message. I am at Pump Expo (stand B-214) until ${fmtDay(dayOnly(14), false)} and will reply when I am back. For urgent matters please contact Mark Ellison, mark@nordwave.example.`, `Спасибо за письмо! До ${fmtDay(dayOnly(14), true)} я на выставке «Насосы-Экспо» (стенд B-214) и отвечу после возвращения. По срочным вопросам — Максим Орлов, maxim@nordwave.example.`), updatedBy: 'anna' },
    ['support@' + D]: { mailbox: 'support@' + D, enabled: true, reason: 'other', dateFrom: '', dateTo: '', substitute: '', notes: '',
      subject: L('We have received your request', 'Ваша заявка получена'), body: L('Thank you — your request has been received and registered. A service engineer will reply within 2 working hours. For emergencies call +44 117 555 0199 (24/7).', 'Спасибо! Ваша заявка получена и зарегистрирована. Сервисный инженер ответит в течение 2 рабочих часов. В экстренных случаях звоните +7 (812) 555-01-99 (круглосуточно).'), updatedBy: 'sofia' },
  }

  // ---------- admin ----------
  const domains = [
    { id: 1, name: D, active: true, createdAt: iso(dayOnly(-640)), dkimSelector: 'tm2025' },
    { id: 2, name: D2, active: true, createdAt: iso(dayOnly(-210)), dkimSelector: 'tm2026' },
  ]
  const IP = '203.0.113.25'
  const HOST = org.host
  const dkimKey = 'MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAr8Y2n0demoKEYonlyB2FkQ7xV1p9Zc3uWm0h4T8bN6sQeLxR5dP2aG7fK1yJ9vH3cE4uM8wZ0tS6iO1nB5qX2rD7lF9gA3kC6jV0hU4yT8mN1pW5sR2eQ9zL7xK3bG6fD4aJ0cI8vY2uO5tH1nM7lE3rS9wP6qA0zX4kB8jC2gF5dV7hU1iN3yT6oL9mR0eW4sQ2aZ8xK5bJ1cG7fD3vH9uY6tI0nO4lM2rE8wP5qS1zA7kX3jB9gC6dF0hV4iU2yT8oN5mL1rW7eQ3sZ9aK6xJ0bG4cD2fH8vY5uI1tO7nM3lR9wE6pS2qZ0aX4kJ8bC5gD1hF7vU3iY9tN6oL2mW8rE4qS0zA5xK1jB7cG3dF9hV6uI2yT8nO4lM0rW6eQ1sZ7aIDAQAB'
  const dnsFor = (name, dmarcMissing) => [
    { key: 'mx', title: 'MX: where mail is delivered', status: 'ok', type: 'MX', name: '@', expected: `10 ${HOST}.`, found: `10 ${HOST}.`, hint: '' },
    { key: 'spf', title: 'SPF: who may send mail', status: 'ok', type: 'TXT', name: '@', expected: 'v=spf1 mx ~all', found: 'v=spf1 mx ~all', hint: '' },
    { key: 'dkim', title: 'DKIM: message signature', status: 'ok', type: 'TXT', name: (name === D ? 'tm2025' : 'tm2026') + '._domainkey', expected: 'v=DKIM1; k=rsa; p=' + dkimKey, found: 'v=DKIM1; k=rsa; p=' + dkimKey, hint: '' },
    dmarcMissing
      ? { key: 'dmarc', title: 'DMARC: policy for forged mail', status: 'missing', type: 'TXT', name: '_dmarc', expected: `v=DMARC1; p=none; rua=mailto:postmaster@${name}; adkim=s; aspf=r`, found: '', hint: 'Gmail and other large providers trust domains without DMARC less.' }
      : { key: 'dmarc', title: 'DMARC: policy for forged mail', status: 'ok', type: 'TXT', name: '_dmarc', expected: `v=DMARC1; p=none; rua=mailto:postmaster@${name}; adkim=s; aspf=r`, found: `v=DMARC1; p=quarantine; rua=mailto:postmaster@${name}; adkim=s; aspf=r`, hint: '' },
    { key: 'a', title: 'A: mail server address', status: 'ok', type: 'A', name: HOST, expected: IP, found: IP, hint: '' },
    { key: 'ptr', title: 'PTR: reverse DNS of the IP', status: 'ok', type: 'PTR', name: IP, expected: HOST + '.', found: HOST + '.', hint: '' },
  ]
  const dns = { 1: dnsFor(D, false), 2: dnsFor(D2, true) }

  let aid = 0
  const alias = (local, domain, dests, active = true, days = 300) => ({ id: ++aid, source: `${local}@${domain}`, destinations: dests, active, domain, createdAt: iso(dayOnly(-days)) })
  const aliases = [
    alias('hello', D, ['info@' + D], true, 600),
    alias('orders', D, ['sales@' + D, U.mark.personal], true, 540),
    alias('service', D, ['support@' + D], true, 500),
    alias('postmaster', D, ['anna@' + D], true, 640),
    alias('invoices', D, ['info@' + D, U.lena.personal], true, 420),
    alias('jobs', D, [U.peter.personal], false, 200),
    alias('parts', D2, ['shop@' + D2], true, 200),
  ]

  const collector = [
    { id: 1, mailbox: gmail, address: gmail, displayName: L('Anna Lindqvist', 'Анна Соколова'), protocol: 'imap', host: 'imap.gmail.example', port: 993, security: 'ssl', username: gmail,
      smtpHost: 'smtp.gmail.example', smtpPort: 465, smtpSecurity: 'ssl', smtpUsername: gmail, hasSmtpPassword: true, inboxOnly: false, since: '', deleteAfter: false, enabled: true,
      status: 'ok', lastError: '', lastRun: iso(back(1)), imported: 9381, interval: 2, twoWay: true, keepDays: 90, remaining: 0, remainingBytes: 0 },
    { id: 2, mailbox: 'office@nordwave-legacy.example', address: 'office@nordwave-legacy.example', displayName: L('Nordwave (old hosting)', 'Нордвейв (старый хостинг)'), protocol: 'pop3', host: 'pop.legacy-host.example', port: 995, security: 'ssl', username: 'office@nordwave-legacy.example',
      smtpHost: '', smtpPort: 465, smtpSecurity: 'ssl', smtpUsername: '', hasSmtpPassword: false, inboxOnly: true, since: '', deleteAfter: true, enabled: true,
      status: 'error', lastError: 'Error: authentication failed', lastRun: iso(back(14)), imported: 21877, interval: 15, twoWay: false, keepDays: 30, remaining: 0, remainingBytes: 0 },
  ]

  let rid = 0
  const rule = (kind, pattern, name, note, by, days) => ({ id: ++rid, kind, pattern, name, note, createdBy: by, createdAt: iso(days ? at(-days, 9 + rid, 10 + rid * 7) : back(25 + rid)) })
  const senderRules = [
    rule('block', '@prize-centre.example', '', L('Prize scam', 'Мошенничество с призами'), 'anna', 40),
    rule('block', '@rank-boost.example', '', L('SEO spam', 'SEO-спам'), 'anna', 31),
    rule('block', 'ap@fastpay-invoices.example', L('Accounts Payable', 'Отдел расчётов'), L('Fake invoices with macros', 'Поддельные счета с макросами'), 'peter', 2),
    rule('block', '@nordwave-secure.example', '', L('Phishing: imitates our IT helpdesk', 'Фишинг: подделка под нашу ИТ-поддержку'), 'anna', 0),
    rule('allow', '@tidewater-shipyards.example', '', L('Customer — never to spam', 'Заказчик — не в спам'), 'anna', 90),
    rule('allow', '@brightline-parts.example', '', L('Supplier invoices', 'Счета поставщика'), 'lena', 120),
    rule('allow', 'news@pumpweekly.example', L('Pump Industry Weekly', 'Насосный вестник'), '', 'mark', 60),
  ]

  const antivirus = {
    ok: true, version: `ClamAV 1.4.2/27803/${clamDate(back(9 * 60))}`, engine: 'ClamAV 1.4.2', signatures: '27803', updated: clamDate(back(9 * 60)), ageHours: 9, stale: false,
    quarantine: [
      { id: 3, mailbox: 'office@nordwave-legacy.example', source: 'collector', sender: 'billing@unknown-supplier.example', subject: L('Payment details updated', 'Изменение реквизитов'), virus: 'Eicar-Signature', size: 18342, created: iso(back(60 * 26)), released: null, releasedBy: '' },
      { id: 2, mailbox: gmail, source: 'collector', sender: 'docs@shared-files.example', subject: L('Shared document: Q3 report.zip', 'Общий документ: отчёт_3кв.zip'), virus: 'Win.Trojan.Agent-1394471', size: 244310, created: iso(back(60 * 24 * 4)), released: null, releasedBy: '' },
      { id: 1, mailbox: 'office@nordwave-legacy.example', source: 'collector', sender: 'scanner@nordwave-legacy.example', subject: L('Scan from office printer', 'Скан с офисного МФУ'), virus: 'Heuristics.Encrypted.Zip', size: 512004, created: iso(back(60 * 24 * 9)), released: iso(back(60 * 24 * 8)), releasedBy: 'anna' },
    ],
    rejected: [
      { time: iso(back(60 * 5)), sender: 'hr@careers-portal.example', rcpt: ['info@' + D], subject: L('CV for the engineer position', 'Резюме на вакансию инженера'), reason: 'virus', virus: 'Doc.Downloader.Emotet-9958043-0' },
      { time: iso(back(60 * 19)), sender: 'delivery@parcel-track.example', rcpt: ['sales@' + D], subject: L('Your parcel could not be delivered', 'Ваша посылка не доставлена'), reason: 'exec', virus: 'label_scan.scr' },
      { time: iso(back(60 * 30)), sender: 'test@' + D, rcpt: ['anna@' + D], subject: 'EICAR test', reason: 'virus', virus: 'Eicar-Signature' },
    ],
  }

  const server = {
    hostname: HOST, publicIp: IP, goVersion: 'go1.24.4',
    certificate: { notAfter: iso(dayOnly(61)), issuer: "R11 (Let's Encrypt)", daysLeft: 61 },
    memory: { total: 16 * 2 ** 30, available: 9.3 * 2 ** 30, swapTotal: 4 * 2 ** 30, swapFree: 3.8 * 2 ** 30 },
    disk: { total: 400 * 2 ** 30, free: 251 * 2 ** 30 },
    load: '0.42 0.37 0.31', cpu: { cores: 8, percent: 7, iowait: 0 }, uptimeSeconds: 41 * 86400 + 5 * 3600,
    antivirus: { ok: true, engine: 'ClamAV 1.4.2', signatures: '27803', updated: iso(back(9 * 60)), ageHours: 9, checked24h: 1284, found24h: 2, quarantine: 2, pendingLinks: 0, rejected24h: 3 },
    queue: { active: 0, deferred: 0 },
    spam: { scanned: 248913, learned: 1764, actions: { reject: 3127, 'add header': 9418, greylist: 2201, 'no action': 234167 } },
    clients: { imap: { host: HOST, port: 993, security: 'SSL/TLS' }, pop3: { host: HOST, port: 995, security: 'SSL/TLS' }, smtp: { host: HOST, port: 465, security: 'SSL/TLS', alt: '587 STARTTLS' } },
  }
  const services = [
    { key: 'smtp', title: 'Incoming mail (SMTP, port 25)', ok: true, detail: `220 ${HOST} ESMTP` },
    { key: 'submission', title: 'Sending (SMTP, port 465)', ok: true, detail: `220 ${HOST} ESMTP` },
    { key: 'imap', title: 'IMAP (port 993)', ok: true, detail: '* OK [CAPABILITY IMAP4rev1 SASL-IR LOGIN-REFERRALS ID ENABLE IDLE LITERAL+ AUTH=PLAIN] Dovecot ready.' },
    { key: 'db', title: 'Database', ok: true, detail: 'responding' },
    { key: 'rspamd', title: 'Rspamd antispam', ok: true, detail: 'messages scanned: 248913' },
    { key: 'clamav', title: 'ClamAV antivirus', ok: true, detail: `databases 27803 of ${clamDate(back(9 * 60))}` },
    { key: 'dns', title: 'DNS resolver', ok: true, detail: 'responding' },
  ]

  // Audit log, newest first.
  const UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36'
  const UAM = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_6) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15'
  const UAP = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148'
  const a = (min, actor, action, target, details = {}, ip = '198.51.100.24') => ({ at: iso(back(min)), actor, action, target, details, ip })
  const auditSrc = [
    a(3, 'anna', 'login.ok', 'anna', { ua: UA }),
    a(41, 'sofia', 'login.ok', 'sofia', { ua: UAP }, '198.51.100.77'),
    a(58, '', 'login.failed', 'admin', { ua: 'python-requests/2.32' }, '192.0.2.181'),
    a(59, '', 'login.failed', 'admin', { ua: 'python-requests/2.32' }, '192.0.2.181'),
    a(61, '', 'login.failed', 'root', { ua: 'python-requests/2.32' }, '192.0.2.181'),
    a(95, 'mark', 'login.ok', W.U.mark.login, { ua: UAM }, '198.51.100.31'),
    a(130, 'anna', 'crm.save', C.tide.name),
    a(170, 'anna', 'ai.run', 'reply', { ms: 1840 }),
    a(240, 'peter', 'senders.block', 'ap@fastpay-invoices.example'),
    a(300, 'anna', 'alias.update', 'orders@' + D, { destinations: ['sales@' + D, U.mark.personal] }),
    a(480, 'anna', 'mailbox.signature', 'anna@' + D),
    a(60 * 20, 'anna', 'user.access', W.U.tom.login, { mailboxes: [{}, {}] }),
    a(60 * 22, 'anna', 'user.update', W.U.tom.login, { active: false }),
    a(60 * 26, 'anna', 'collector.save', 'office@nordwave-legacy.example'),
    a(60 * 30, 'lena', 'login.ok', W.U.lena.login, { ua: UA }, '198.51.100.52'),
    a(60 * 49, 'anna', 'domain.create', D2),
    a(60 * 50, 'anna', 'mailbox.create', 'shop@' + D2),
    a(60 * 51, 'anna', 'user.create', W.U.tom.login, { admin: false, mailboxes: 2 }),
    a(60 * 72, 'sofia', 'autoreply.save', 'support@' + D),
    a(60 * 75, 'peter', 'login.ok', W.U.peter.login, { ua: UAM }, '198.51.100.12'),
    a(60 * 96, 'anna', 'ai.provider', 'openrouter', { url: 'https://openrouter.ai/api', model: 'openai/gpt-4o-mini' }),
    a(60 * 100, 'anna', 'folder.save', 'anna@' + D),
    a(60 * 120, 'anna', 'user.password', W.U.sofia.login),
  ]
  const loginOf = k => (U[k] ? U[k].login : k)
  const audit = auditSrc.map((x, i) => ({ id: 5000 - i, ...x, actor: loginOf(x.actor) }))
  const sentLog = [
    [12, 'anna', 'anna@' + D, L('Re: Tidewater — let\'s agree on the discount before Friday', 'Re: Верфь — согласуем скидку до пятницы'), [P.peter.email, P.mark.email]],
    [60 * 2, 'mark', 'sales@' + D, L('Re: PO 4471 — 6 electric valve actuators', 'Re: Заказ № 4471 — 6 электроприводов'), [P.daniel.email, P.oliver.email]],
    [60 * 5, 'sofia', 'support@' + D, L('Re: URGENT: KP-200 vibrates after restart', 'Re: СРОЧНО: КП-200 вибрирует после перезапуска'), [P.ruben.email]],
    [60 * 7, 'anna', 'sales@' + D, L('Re: Partnership proposal: cast steel valve bodies', 'Re: Предложение о сотрудничестве: стальные литые корпуса'), [P.piotr.email]],
    [60 * 9, 'anna', 'anna@' + D, L('KP-300 drawings and 3D model', 'Чертежи и 3D-модель КП-300'), [P.femke.email]],
    [60 * 20, 'anna', gmail, L('Re: Dinner on Saturday?', 'Re: Ужин в субботу?'), [P.friend.email]],
  ].map(([min, login, mailbox, subject, recipients], i) => ({ id: 900 - i, at: iso(back(min)), login: login === 'mark' ? U.mark.login : login === 'sofia' ? U.sofia.login : login, mailbox, subject, recipients }))

  const mcpTokens = [
    { id: 3, name: L('Claude Desktop — laptop', 'Claude Desktop — ноутбук'), prefix: 'tm_8f3kq2Lx', canSend: false, canModify: true, mailboxes: [], created: iso(back(60 * 100)), lastUsed: iso(back(35)), expires: iso(dayOnly(265)), login: 'anna' },
    { id: 2, name: L('Weekly report agent', 'Агент еженедельного отчёта'), prefix: 'tm_c41Zp7Qw', canSend: false, canModify: false, mailboxes: ['sales@' + D], created: iso(dayOnly(-40)), lastUsed: iso(back(60 * 24 * 3)), expires: null, login: 'anna' },
    { id: 1, name: L('Support triage bot', 'Бот разбора заявок'), prefix: 'tm_9Hd2mVt0', canSend: true, canModify: true, mailboxes: ['support@' + D], created: iso(dayOnly(-75)), lastUsed: iso(back(18)), expires: iso(dayOnly(290)), login: U.sofia.login },
  ]

  // ---------- AI assistant ----------
  const prompt = (key, title, description, system, template, extra = {}) => ({ key, title, description, system, template, model: '', temperature: 0.3, maxTokens: 1200, enabled: true, updatedAt: '', updatedBy: '', ...extra })
  const prompts = [
    prompt('reply', L('Reply with AI', 'Ответ с ИИ'), L('Drafts a reply to the open letter. Available as a button under the letter and in the reply window.', 'Пишет черновик ответа на открытое письмо. Кнопка под письмом и в окне ответа.'),
      L('You are an assistant who helps a company employee reply to business letters from customers and partners. Write politely, briefly and to the point.\nRules:\n- Reply in the language of the letter.\n- Do not invent prices, dates or promises that are not in the instructions.\n- No subject line and no signature.', 'Ты помощник сотрудника компании, который отвечает на деловые письма клиентов и партнёров. Пиши вежливо, коротко и по делу.\nПравила:\n- Отвечай на языке письма.\n- Не выдумывай цены, сроки и обещания, которых нет в указаниях.\n- Без темы и без подписи.'),
      L('Letter from {from} to {to}, subject "{subject}":\n{letter}\n\nEarlier correspondence:\n{thread}\n\nWhat the employee wants to say: {instructions}\nDraft so far: {draft}\nEmployee: {user}', 'Письмо от {from} для {to}, тема «{subject}»:\n{letter}\n\nПредыдущая переписка:\n{thread}\n\nЧто хочет сказать сотрудник: {instructions}\nЧерновик: {draft}\nСотрудник: {user}'), { updatedAt: iso(back(60 * 96)), updatedBy: 'anna' }),
    prompt('reply_questions', L('Questions in the letter', 'Вопросы в письме'), L('Before replying with AI: finds the questions and requests in the letter, suggests short answer options and what to ask the sender.', 'Перед ответом с ИИ: находит вопросы и просьбы в письме, предлагает короткие варианты ответов и что уточнить у отправителя.'),
      L('You help a company employee reply to business letters. Find every question and request in the latest letter and answer with JSON: {"intent": "...", "questions": [{"question": "...", "options": ["..."]}], "clarify": ["..."]}', 'Ты помогаешь сотруднику отвечать на деловые письма. Найди все вопросы и просьбы в последнем письме и ответь JSON: {"intent": "...", "questions": [{"question": "...", "options": ["..."]}], "clarify": ["..."]}'),
      '{letter}\n\n{thread}\n\n{attachments}', { temperature: 0.2 }),
    prompt('summary', L('Summarize letter', 'Кратко о письме'), L('A short summary of a long letter: what they want, deadlines, amounts, what needs to be done.', 'Коротко о длинном письме: чего хотят, сроки, суммы, что нужно сделать.'),
      L('Summarize the letter in 3–5 short lines: who writes, what they want, deadlines and amounts, what we need to do.', 'Перескажи письмо в 3–5 коротких строках: кто пишет, чего хочет, сроки и суммы, что нам нужно сделать.'), '{letter}'),
    prompt('write', L('Write a letter', 'Написать письмо'), L('A new letter from your instructions (they can be dictated by voice).', 'Новое письмо по вашим указаниям (можно надиктовать голосом).'),
      L('You are an assistant who helps a company employee write business letters: polite, clear and brief.', 'Ты помогаешь сотруднику компании писать деловые письма: вежливо, ясно и коротко.'), L('To: {to}\nSubject: {subject}\nInstructions: {instructions}\nDraft: {draft}', 'Кому: {to}\nТема: {subject}\nУказания: {instructions}\nЧерновик: {draft}')),
    prompt('improve', L('Improve text', 'Улучшить текст'), L('Rewrites what you have already written: fixes mistakes and makes the text clear and polite.', 'Переписывает уже написанное: исправляет ошибки, делает текст ясным и вежливым.'),
      L('Improve the letter text: fix mistakes, make it clear and polite. Keep the meaning and the language.', 'Улучши текст письма: исправь ошибки, сделай ясным и вежливым. Сохрани смысл и язык.'), '{draft}\n\n{instructions}'),
    prompt('fix', L('Fix mistakes', 'Исправить ошибки'), L('Right-click in the letter text: fixes spelling, punctuation and typos, changing nothing else.', 'Правая кнопка в тексте письма: исправляет орфографию, пунктуацию и опечатки, больше ничего не меняя.'),
      L('You are a proofreader. Fix only spelling, punctuation and grammar mistakes and typos. Return only the corrected text.', 'Ты корректор. Исправь только орфографические, пунктуационные и грамматические ошибки и опечатки. Верни только исправленный текст.'), '{draft}', { temperature: 0.1 }),
    prompt('rephrase', L('Rephrase', 'Перефразировать'), L('Right-click in the letter text: the same meaning in other words.', 'Правая кнопка в тексте письма: тот же смысл другими словами.'), L('Rephrase the text, keeping its meaning and language.', 'Перефразируй текст, сохранив смысл и язык.'), '{draft}'),
    prompt('dictate', L('Tidy up dictation', 'Причесать диктовку'), L('Turns text dictated by voice into a neat letter text.', 'Превращает надиктованный текст в аккуратный текст письма.'), L('Turn dictated speech into clean written text.', 'Преврати надиктованную речь в аккуратный письменный текст.'), '{draft}'),
    prompt('pick_template', L('Pick a template', 'Подобрать шаблон'), L('Chooses a suitable reply template for the open letter.', 'Выбирает подходящий шаблон ответа для открытого письма.'), L('Answer only with the number of the most suitable template, or 0.', 'Ответь только номером самого подходящего шаблона или 0.'), 'Letter:\n{letter}\n\nTemplates:\n{templates}', { temperature: 0, maxTokens: 50 }),
    prompt('autoreply', L('Auto-reply text', 'Текст автоответа'), L('Writes the auto-reply text from the reason and the dates chosen by the employee.', 'Пишет текст автоответа по причине и датам, которые выбрал сотрудник.'), L('You write a short, polite auto-reply for a business mailbox. 3–5 sentences.', 'Ты пишешь короткий вежливый автоответ для делового ящика. 3–5 предложений.'), 'Reason for absence: {instructions}'),
    prompt('sort', L('Sort into folders', 'Раскладка по папкам'), L('Decides which folder a new incoming letter goes to (for folders with automatic sorting turned on).', 'Решает, в какую папку положить новое письмо (для папок с автоматической раскладкой).'), L('Choose the folder for the letter from the list, or "none".', 'Выбери папку для письма из списка или «none».'), '{folders}\n\n{letter}', { temperature: 0, maxTokens: 50 }),
    prompt('extract', L('Counterparty cards', 'Карточки контрагентов'), L('Takes the sender\'s details from a letter and its attachments: company, identifiers, bank details, contact person, phones, delivery addresses.', 'Берёт реквизиты отправителя из письма и вложений: компания, идентификаторы, банк, контактное лицо, телефоны, адреса доставки.'), L('You extract counterparty details from business correspondence. Answer with JSON.', 'Ты извлекаешь реквизиты контрагентов из деловой переписки. Ответь JSON.'), '{letter}\n\n{attachments}', { temperature: 0, maxTokens: 2000 }),
  ]
  const aiStatus = { configured: true, name: 'Loggi', kind: 'openrouter', model: 'openai/gpt-4o-mini', host: 'openrouter.ai', secure: true, ms: 312, reachable: true,
    models: ['anthropic/claude-sonnet-4.5', 'deepseek/deepseek-chat-v3.1', 'google/gemini-2.5-flash', 'meta-llama/llama-3.3-70b-instruct', 'mistralai/mistral-small-3.2-24b-instruct', 'openai/gpt-4.1-mini', 'openai/gpt-4o', 'openai/gpt-4o-mini', 'qwen/qwen3-235b-a22b'] }
  const aiProvider = { provider: { kind: 'openrouter', name: 'Loggi', url: 'https://openrouter.ai/api', model: 'openai/gpt-4o-mini', asrUrl: 'https://api.openai.com', asrModel: 'whisper-1' },
    hasKey: true, hasAsrKey: true, fromEnv: false, asrActive: true, asrModelInUse: 'whisper-1',
    presets: { openai: { kind: 'openai', name: '', url: 'https://api.openai.com', model: 'gpt-4o-mini', asrUrl: '', asrModel: 'whisper-1' }, openrouter: { kind: 'openrouter', name: '', url: 'https://openrouter.ai/api', model: 'openai/gpt-4o-mini', asrUrl: '', asrModel: '' } } }

  return { labels, ownFolders, folderRules, events, due, plan, templates, autoreply, domains, dns, aliases, collector, senderRules, antivirus, server, services, audit, sentLog, mcpTokens, prompts, aiStatus, aiProvider }
}

function nextDow(now, dow) { // days until the next given weekday (1 = Monday), never today
  const d = new Date(now).getDay()
  let n = (dow - d + 7) % 7
  return n === 0 ? 7 : n
}
function ymd(d) { return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}` }
function fmtDay(d, ru) { return new Intl.DateTimeFormat(ru ? 'ru-RU' : 'en-GB', { day: 'numeric', month: 'long' }).format(d) }
function clamDate(d) { return d.toUTCString().replace(/^(\w+), (\d+) (\w+) (\d+) (\S+) GMT$/, (m, w, day, mon, y, t) => `${w} ${mon} ${day.padStart(2, ' ')} ${t} ${y}`) }
