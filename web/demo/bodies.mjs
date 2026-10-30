// Letters whose body is real HTML (a newsletter, a phishing attempt, a price table, a web form).
// {{ORIGIN}} becomes the demo server's address; {{REMOTE}} another origin, so the pictures behave
// like remote ones (blocked until "Show pictures").

export function bodies(W) {
  const { L } = W
  const newsletter = `<table width="100%" cellpadding="0" cellspacing="0" style="background:#eef2f6;font-family:Arial,Helvetica,sans-serif"><tr><td align="center" style="padding:20px 10px">
<table width="600" cellpadding="0" cellspacing="0" style="background:#ffffff;border-radius:8px;overflow:hidden">
<tr><td><img src="{{REMOTE}}/demo-remote/news-banner.png" width="600" height="200" alt="${L('Pump Industry Weekly', 'Насосный вестник')}" style="display:block"></td></tr>
<tr><td style="padding:24px 28px;color:#1d2733;font-size:15px;line-height:1.55">
<h1 style="font-size:22px;margin:0 0 12px">${L('5 trends in seawater pumps for 2027', '5 трендов в насосах для морской воды')}</h1>
<p>${L('Shipbuilders are ordering more pumps than ever. Here is what buyers will ask for next year:', 'Верфи заказывают насосов больше, чем когда-либо. Вот что покупатели будут спрашивать в следующем году:')}</p>
<ol style="padding-left:20px">
<li>${L('<b>Duplex and super-duplex</b> instead of bronze — longer life in warm seawater.', '<b>Дуплекс и супердуплекс</b> вместо бронзы — дольше служат в тёплой морской воде.')}</li>
<li>${L('<b>Variable speed drives</b> as standard: up to 40% less energy.', '<b>Частотные приводы</b> как стандарт: до 40% экономии энергии.')}</li>
<li>${L('<b>Condition monitoring</b> — vibration sensors on every pump.', '<b>Мониторинг состояния</b> — датчики вибрации на каждом насосе.')}</li>
<li>${L('<b>Shorter lead times</b> — 4 to 6 weeks is the new normal.', '<b>Короткие сроки поставки</b> — 4–6 недель становятся нормой.')}</li>
<li>${L('<b>Spare parts kits</b> included in the first order.', '<b>Комплекты ЗИП</b> сразу в первом заказе.')}</li>
</ol>
<table cellpadding="0" cellspacing="0" width="100%" style="margin:18px 0;background:#f4f7fb;border-radius:6px"><tr>
<td width="260"><img src="{{REMOTE}}/demo-remote/news-product.png" width="260" height="180" alt="" style="display:block;border-radius:6px 0 0 6px"></td>
<td style="padding:14px 18px;font-size:14px"><b>${L('Product of the week', 'Новинка недели')}</b><br>${L('A compact 15 kW inline pump with an integrated drive and Bluetooth monitoring.', 'Компактный линейный насос 15 кВт со встроенным частотником и мониторингом по Bluetooth.')}<br><br><a href="https://pumpweekly.example/product-of-the-week" style="color:#0a6fb5">${L('Read more →', 'Подробнее →')}</a></td></tr></table>
<p style="text-align:center;margin:26px 0 8px"><a href="https://pumpweekly.example/issue-412" style="background:#0a6fb5;color:#fff;text-decoration:none;padding:12px 26px;border-radius:5px;font-weight:bold">${L('Read the full issue', 'Читать весь выпуск')}</a></p>
</td></tr>
<tr><td style="padding:16px 28px;background:#f4f6f9;color:#7a8592;font-size:12px">${L('You receive this newsletter because you subscribed at pumpweekly.example.', 'Вы получили это письмо, потому что подписались на pumpweekly.example.')} <a href="https://pumpweekly.example/unsubscribe" style="color:#7a8592">${L('Unsubscribe', 'Отписаться')}</a></td></tr>
</table></td></tr></table>`

  const phishing = `<div style="font-family:Arial,Helvetica,sans-serif;font-size:15px;color:#202124;max-width:560px">
<div style="background:#0a4ff5;color:#fff;padding:14px 18px;font-weight:bold;font-size:17px">${L('Nordwave Mail — Security Notice', 'Почта Нордвейв — уведомление безопасности')}</div>
<div style="padding:18px;border:1px solid #dfe3e8;border-top:0">
<p>${L('Dear user,', 'Уважаемый пользователь!')}</p>
<p>${L('We detected unusual sign-in activity on your mailbox. To avoid suspension, you must verify your account <b>within 24 hours</b>.', 'Мы обнаружили подозрительную активность в вашем почтовом ящике. Чтобы избежать блокировки, подтвердите учётную запись <b>в течение 24 часов</b>.')}</p>
<p>${L('Verify here:', 'Подтвердите здесь:')} <a href="http://nordwave-secure.example/login/verify.php?u=anna">https://mail.nordwave.example/verify</a></p>
<p>${L('Or download the security update for your mail client:', 'Или скачайте обновление безопасности для почтовой программы:')} <a href="http://nordwave-secure.example/files/MailSecurityUpdate.zip">MailSecurityUpdate.zip</a></p>
<p style="color:#5f6368;font-size:13px">${L('If you do not verify, your mailbox and all messages will be permanently deleted.', 'Если вы не подтвердите учётную запись, ящик и все письма будут удалены без возможности восстановления.')}</p>
<p>${L('IT Helpdesk', 'Служба ИТ-поддержки')}</p></div></div>`

  const rows = L([
    ['ZV-150-16-C', 'Gate valve DN150 PN16, ductile iron, resilient seat', 24, 'EUR 312.00'],
    ['ZV-150-16-S', 'Gate valve DN150 PN16, cast steel, metal seat', 12, 'EUR 448.00'],
    ['ZV-150-AK', 'Actuator adapter kit F10', 36, 'EUR 46.50'],
  ], [
    ['ЗВ-150-16-Ч', 'Задвижка DN150 PN16, ВЧШГ, обрезиненный клин', 24, '27 900 ₽'],
    ['ЗВ-150-16-С', 'Задвижка DN150 PN16, сталь 25Л, металл/металл', 12, '39 800 ₽'],
    ['ЗВ-150-АК', 'Комплект адаптера под привод F10', 36, '4 150 ₽'],
  ])
  const gateValves = `<div style="font-family:Arial,Helvetica,sans-serif;font-size:14px;color:#1d2733">
<p>${L('Good afternoon,', 'Добрый день!')}</p>
<p>${L('Following your enquiry, here is our offer for DN150 gate valves. All items are in stock in St Petersburg.', 'По вашему запросу направляем предложение на задвижки DN150. Всё в наличии на складе в Санкт-Петербурге.')}</p>
<table cellpadding="6" cellspacing="0" style="border-collapse:collapse;font-size:13px;margin:10px 0">
<tr style="background:#e8eef6"><th align="left" style="border:1px solid #c9d3df">${L('Code', 'Артикул')}</th><th align="left" style="border:1px solid #c9d3df">${L('Description', 'Наименование')}</th><th style="border:1px solid #c9d3df">${L('Qty', 'Кол-во')}</th><th align="right" style="border:1px solid #c9d3df">${L('Price', 'Цена с НДС')}</th></tr>
${rows.map(r => `<tr><td style="border:1px solid #dde3ea">${r[0]}</td><td style="border:1px solid #dde3ea">${r[1]}</td><td align="center" style="border:1px solid #dde3ea">${r[2]}</td><td align="right" style="border:1px solid #dde3ea">${r[3]}</td></tr>`).join('')}
</table>
<p>${L('Delivery: 5 working days. Payment: 100% in advance. The offer is valid until the end of the month.', 'Поставка: 5 рабочих дней. Оплата: 100% предоплата. Предложение действует до конца месяца.')}</p>
<p>${L('Best regards,<br>Dmitry Volkov<br>Sales Director, Severnaya Armatura LLC<br>+7 (812) 555-77-30', 'С уважением,<br>Дмитрий Волков<br>коммерческий директор ООО «Северная Арматура»<br>+7 (812) 555-77-30')}</p></div>`

  const webForm = `<div style="font-family:Arial,Helvetica,sans-serif;font-size:14px;color:#1d2733">
<p style="color:#5f6368">${L('A new message from the contact form on nordwave.example:', 'Новое сообщение из формы обратной связи на nordwave.example:')}</p>
<table cellpadding="5" cellspacing="0" style="font-size:14px">
<tr><td style="color:#5f6368">${L('Name', 'Имя')}</td><td><b>${L('John Carter', 'Иван Карпов')}</b></td></tr>
<tr><td style="color:#5f6368">${L('Company', 'Компания')}</td><td>${L('Carter Marine Services', 'ИП Карпов (судоремонт)')}</td></tr>
<tr><td style="color:#5f6368">E-mail</td><td>${L('john.carter@mailbox.example', 'i.karpov@mailbox.example')}</td></tr>
<tr><td style="color:#5f6368">${L('Phone', 'Телефон')}</td><td>${L('+44 7700 555 913', '+7 (911) 555-09-13')}</td></tr>
</table>
<p style="border-left:3px solid #0a4ff5;padding-left:10px">${L('Hello, do you have 2 × KP-300/45 in stock? We need them for a repair in Avonmouth within two weeks. What would be the price and delivery time?', 'Здравствуйте! Есть ли в наличии 2 насоса КП-300/45? Нужны для ремонта судна в течение двух недель. Какая цена и срок поставки?')}</p></div>`

  return { newsletter, phishing, gateValves, webForm }
}
