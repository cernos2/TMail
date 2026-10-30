// The demo's letters. Dates are relative to "now", so the mailbox always looks current.
// Each letter: id, acct (mailbox), folder (role or own folder name), from/to (people keys or
// addresses), at, subject, text (plain paragraphs) or html, atts, flags… `re` names the letter it
// answers (threading and the quoted text come from it).

export function letters(W, now) {
  const { L, D, P, gmail } = W
  const A = 'anna@' + D, S = 'sales@' + D, SU = 'support@' + D, I = 'info@' + D, G = gmail
  const day = (n, h, m) => { const d = new Date(now); d.setDate(d.getDate() - n); d.setHours(h, m, 7 + (h * 7 + m) % 50, 0); return d }
  const ago = min => new Date(now.getTime() - min * 60000)
  const att = (file, name) => ({ file, name })
  const out = []
  const M = x => { out.push(x); return x }

  // ===================== anna@ — Tidewater tender (a conversation of five letters) =====================
  M({ id: 'tw1', acct: A, folder: 'inbox', from: 'femke', to: [A], at: day(6, 10, 12), seen: true, labels: [2],
    subject: L('Request for quotation: 40 seawater pumps for HT-40 tugs', 'Запрос КП: 40 насосов забортной воды для буксиров'),
    text: L(`Dear Anna,

We are preparing the equipment package for our new series of harbour tugs (HT-40) and would like to invite Nordwave to quote for the seawater cooling pumps.

Quantity: 40 pumps, delivery to our Rotterdam yard by week 50.
The full requirements are in the attached specification (SPEC-2026-114, rev. B).

Could you please send your offer with price, lead time, warranty and payment terms by the end of next week?

Kind regards,
Femke de Vries
Procurement Manager
Tidewater Shipyards B.V.
+31 10 555 2318`, `Добрый день, Анна!

Мы комплектуем оборудование для новой серии буксиров (проект 22800) и приглашаем «Нордвейв» дать предложение на насосы забортной воды.

Количество: 40 шт., поставка на верфь не позднее 50-й недели.
Полные требования — во вложении (ТТ-2026-114, ред. Б).

Просим прислать коммерческое предложение с ценой, сроком поставки, гарантией и условиями оплаты до конца следующей недели.

С уважением,
Елена Фролова
начальник отдела закупок
АО «Балтийская Верфь Северо-Запад»
+7 (812) 555-23-18`),
    atts: [att('spec', L('SPEC-2026-114_seawater_pumps.pdf', 'ТТ-2026-114_насосы_забортной_воды.pdf'))] })
  M({ id: 'tw2', acct: A, folder: 'sent', from: A, to: ['femke'], at: day(6, 15, 40), seen: true, re: 'tw1',
    subject: L('Re: Request for quotation: 40 seawater pumps for HT-40 tugs', 'Re: Запрос КП: 40 насосов забортной воды для буксиров'),
    text: L(`Dear Femke,

Thank you for the invitation — we will be glad to quote.

Two quick questions before we finalise the offer:
1. Do you require DNV class approval for every pump, or a type approval is enough?
2. Is the duty point 120 m³/h at 45 m the same for all 40 units?

You will have our offer by Wednesday.

Best regards,`, `Елена, добрый день!

Спасибо за приглашение, с удовольствием подготовим предложение.

Два вопроса, чтобы не ошибиться:
1. Нужно ли свидетельство РМРС на каждый насос или достаточно типового одобрения?
2. Рабочая точка 120 м³/ч при 45 м одинакова для всех 40 насосов?

Предложение пришлём к среде.

С уважением,`) })
  M({ id: 'tw3', acct: A, folder: 'inbox', from: 'femke', to: [A], at: day(5, 9, 5), seen: true, re: 'tw2', labels: [2],
    subject: L('Re: Request for quotation: 40 seawater pumps for HT-40 tugs', 'Re: Запрос КП: 40 насосов забортной воды для буксиров'),
    text: L(`Hi Anna,

1. Type approval is fine, plus a certificate of conformity for each pump.
2. Yes, the same duty point for all units.

Looking forward to your offer.

Femke`, `Анна, здравствуйте!

1. Достаточно типового одобрения и паспорта на каждый насос.
2. Да, рабочая точка одна для всех.

Ждём предложение.

Елена`) })
  M({ id: 'tw4', acct: A, folder: 'sent', from: A, to: ['femke'], cc: ['peter'], at: day(1, 17, 22), seen: true, re: 'tw3',
    subject: L('Re: Request for quotation: 40 seawater pumps for HT-40 tugs', 'Re: Запрос КП: 40 насосов забортной воды для буксиров'),
    text: L(`Dear Femke,

Please find attached our commercial offer Q-2611 for 40 KP-300/45 pumps in duplex stainless steel.

In short:
• EUR 6,850 per pump, delivered DAP Rotterdam
• lead time 6–7 weeks from the down payment
• 24 months warranty from commissioning
• commissioning supervision by our engineer included as an option

I am happy to go through the details on a call this week.

Best regards,`, `Елена, добрый день!

Направляю коммерческое предложение № 2611 на 40 насосов КП-300/45 в исполнении из дуплексной стали.

Коротко:
• 612 000 ₽ за насос с НДС, доставка на верфь включена
• срок поставки 6–7 недель с даты предоплаты
• гарантия 24 месяца с даты пуска
• шеф-монтаж нашим инженером — опционально

Готова обсудить детали по телефону на этой неделе.

С уважением,`),
    atts: [att('offer', L('Nordwave_Offer_Q-2611.pdf', 'КП_Нордвейв_2611.pdf'))] })
  M({ id: 'tw5', acct: A, folder: 'inbox', from: 'femke', to: [A], cc: ['ruben'], at: ago(95), seen: false, flagged: true, re: 'tw4', labels: [2, 1],
    subject: L('Re: Request for quotation: 40 seawater pumps for HT-40 tugs', 'Re: Запрос КП: 40 насосов забортной воды для буксиров'),
    text: L(`Dear Anna,

Thank you for the offer, it looks good. Before we present it to our technical committee on Monday, a few questions:

1. Could you shorten the lead time to 4 weeks for the first 10 pumps?
2. What discount can you offer if we order all 40 pumps at once?
3. Would you accept payment 30/70 instead of 50/50?
4. Can you include a spare parts kit (seals and bearings) for every 10 pumps?

Could we have a short call tomorrow afternoon?

Best regards,
Femke de Vries
Procurement Manager
Tidewater Shipyards B.V.`, `Анна, добрый день!

Спасибо за предложение, выглядит хорошо. Перед техническим советом в понедельник есть несколько вопросов:

1. Можно ли сократить срок до 4 недель для первых 10 насосов?
2. Какую скидку дадите, если закажем все 40 насосов сразу?
3. Возможна ли оплата 30/70 вместо 50/50?
4. Можете включить комплект ЗИП (уплотнения и подшипники) на каждые 10 насосов?

Удобно созвониться завтра после обеда?

С уважением,
Елена Фролова
начальник отдела закупок
АО «Балтийская Верфь Северо-Запад»`) })

  // ===================== anna@ — other letters =====================
  M({ id: 'pt1', acct: A, folder: 'inbox', from: 'peter', to: [A], cc: ['mark'], at: day(1, 9, 30), seen: true, answered: true,
    subject: L('Tidewater — let\'s agree on the discount before Friday', 'Верфь — согласуем скидку до пятницы'),
    text: L(`Anna, Mark,

Tidewater will almost certainly ask for a volume discount. My limit is 4% on the pumps, and up to 6% if they pay 50% upfront. The spare parts kit we can give at cost.

Let's meet on Tuesday at 10:00 in my office to agree on the final terms.

Peter`, `Анна, Максим,

Верфь наверняка попросит скидку за объём. Мой предел — 4% на насосы и до 6%, если они платят 50% вперёд. Комплект ЗИП можем отдать по себестоимости.

Давайте во вторник в 10:00 у меня обсудим финальные условия.

Пётр`) })
  M({ id: 'pt2', acct: A, folder: 'sent', from: A, to: ['peter'], cc: ['mark'], at: day(1, 10, 5), seen: true, re: 'pt1',
    subject: L('Re: Tidewater — let\'s agree on the discount before Friday', 'Re: Верфь — согласуем скидку до пятницы'),
    text: L(`Peter, agreed — Tuesday 10:00 works. I'll bring the cost calculation for the spare parts kits.`, `Пётр, договорились, вторник 10:00. Принесу расчёт себестоимости комплектов ЗИП.`) })
  M({ id: 'news1', acct: A, folder: 'inbox', from: 'news', to: [A], at: day(1, 7, 0), seen: false, remote: true, dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
    subject: L('5 trends in seawater pumps for 2027 · Pump Industry Weekly #412', '5 трендов в насосах для морской воды · Насосный вестник № 412'),
    html: 'newsletter' })
  M({ id: 'ph1', acct: A, folder: 'inbox', from: 'phish', to: [A], at: ago(40), seen: false, auth: { spf: 'fail', dkim: 'fail', dmarc: 'fail' }, dmarc: 'fail',
    subject: L('Action required: your mailbox will be suspended today', 'Срочно: ваш почтовый ящик будет заблокирован сегодня'),
    html: 'phishing' })
  M({ id: 'cl1', acct: A, folder: 'inbox', from: 'clara', to: [A], at: day(8, 16, 20), seen: true, answered: true,
    subject: L('KP-300 manual — German translation is ready', 'Руководство КП-300 — перевод на английский готов'),
    text: L(`Hi Anna,

The German translation of the KP-300 operating manual is attached (version 3.2). I kept the terminology from the KP-200 manual, so the two documents match.

The invoice will follow separately. Let me know if anything needs changing.

Best,
Clara Mendes
Technical translator`, `Анна, добрый день!

Во вложении перевод руководства по эксплуатации КП-300 на английский (версия 3.2). Терминологию взяла из руководства КП-200, чтобы документы совпадали.

Счёт пришлю отдельно. Если нужно что-то поправить — напишите.

Ирина Лаптева
технический переводчик`),
    atts: [att('manual', L('KP-300_Betriebsanleitung_DE.docx', 'KP-300_Operating_Manual_EN.docx'))] })
  M({ id: 'cl2', acct: A, folder: 'sent', from: A, to: ['clara'], at: day(8, 17, 5), seen: true, re: 'cl1',
    subject: L('Re: KP-300 manual — German translation is ready', 'Re: Руководство КП-300 — перевод на английский готов'),
    text: L(`Thank you, Clara — great work and right on time. Please send the invoice to lena@nordwave.example.`, `Ирина, спасибо, отличная работа и точно в срок. Счёт, пожалуйста, на elena@nordwave.example.`) })
  M({ id: 'ks1', acct: A, folder: 'inbox', from: 'rachel', to: [A], cc: ['peter'], at: day(3, 14, 10), seen: true, flagged: true, labels: [4],
    subject: L('Distribution agreement — draft for your review', 'Дилерский договор — проект на согласование'),
    text: L(`Hello Anna,

As discussed in Bristol, please find attached the draft distribution agreement for the UK and Ireland.

The main points:
• non-exclusive distribution, 2 years with automatic renewal
• distributor discount 22% off the list price
• minimum purchase USD 240,000 per year
• payment 30 days from shipment

Please send us your comments by the 15th, so we can sign at Pump Expo.

Kind regards,
Rachel Kim
Channel Manager EMEA
Kestrel Pump Systems Inc.`, `Анна, добрый день!

Как договаривались, направляю проект дилерского договора на Северо-Западный округ.

Основные условия:
• неэксклюзивное дилерство, 2 года с автопролонгацией
• дилерская скидка 22% от прайс-листа
• минимальный объём закупок 18 млн рублей в год
• оплата в течение 30 дней с даты отгрузки

Комментарии ждём до 15-го числа, чтобы подписать договор на выставке.

С уважением,
Ержан Нурланов
директор
ТОО «Кестрел Пампс Евразия»`),
    atts: [att('contract', L('Kestrel_Nordwave_Distribution_Agreement_DRAFT.docx', 'Договор_Кестрел_Нордвейв_проект.docx'))] })
  M({ id: 'so1', acct: A, folder: 'inbox', from: 'sofia', to: [A], at: day(2, 12, 15), seen: true,
    subject: L('Tidewater KP-200 vibration — site visit on Friday?', 'Вибрация КП-200 на верфи — выезд в пятницу?'),
    text: L(`Anna,

The vibration on Tidewater's KP-200 in pump room 3 looks like a misalignment after the motor was replaced. I'd like to go there on Friday with the laser alignment kit.

It is under warranty, so no charge — and a good moment to talk to Ruben about the new tugs.

Sofia`, `Анна,

Вибрация на КП-200 в насосной № 3 на верфи похожа на расцентровку после замены двигателя. Хочу поехать в пятницу с лазерным центровщиком.

Насос на гарантии, так что бесплатно — и заодно хороший повод поговорить с Романом про новые буксиры.

София`) })
  M({ id: 'le1', acct: A, folder: 'inbox', from: 'lena', to: [A], at: day(4, 10, 2), seen: true,
    subject: L('Brightline invoice 418 — OK to pay?', 'Счёт «Брайтлайн Партс» № 418 — оплачиваем?'),
    text: L(`Hi Anna,

Brightline's invoice INV-2026-0418 (EUR 12,016.00) is due on the 25th. The quantities match our order NW-PO-2611. Shall I pay it this week?

Lena`, `Анна, привет!

Счёт № 418 от «Брайтлайн Партс» на 1 075 300 ₽, срок оплаты — 25-е. Количество совпадает с нашим заказом НВ-2611. Оплачиваем на этой неделе?

Елена`) })
  M({ id: 'big1', acct: A, folder: 'sent', from: A, to: ['femke'], at: day(3, 11, 0), seen: true,
    subject: L('KP-300 drawings and 3D model', 'Чертежи и 3D-модель КП-300'),
    text: L(`Dear Femke,

As promised, here are the assembly drawings and the 3D model of the KP-300/45 for your design office. The files are too large for e-mail, so they come as download links.`, `Елена, добрый день!

Как обещала, направляю сборочные чертежи и 3D-модель КП-300/45 для вашего КБ. Файлы слишком большие для письма, поэтому — ссылками.`),
    links: [{ token: 'kp300-3d-model', name: 'KP-300_45_assembly.step', size: 193462272, mime: 'application/step' }, { token: 'kp300-drawings', name: L('KP-300_45_drawings_rev_C.pdf', 'КП-300_45_чертежи_ред_C.pdf'), size: 39845888, mime: 'application/pdf', thumb: true }] })
  M({ id: 'dr1', acct: A, folder: 'drafts', from: A, to: ['markus'], at: ago(200), seen: true,
    subject: L('Spare impellers for the Kestrel pumps', 'Запасные рабочие колёса для насосов'),
    text: L(`Hello Markus,

We need a quote for 12 spare impellers, 240 mm, bronze, for our KP-300 service stock. Could you`, `Маркус, добрый день!

Нам нужно предложение на 12 запасных рабочих колёс диаметром 240 мм из бронзы для сервисного склада КП-300. Не могли бы вы`) })
  // Archive
  M({ id: 'ar1', acct: A, folder: 'archive', from: 'peter', to: [A], at: day(24, 18, 2), seen: true,
    subject: L('Q3 sales report — well done', 'Итоги продаж за III квартал — отлично'),
    text: L(`Anna,

Q3 closed at 112% of plan, the best quarter in three years. Please pass my thanks to the team. Let's plan the Q4 pipeline review next week.

Peter`, `Анна,

III квартал закрыли на 112% от плана — лучший квартал за три года. Передай команде мою благодарность. На следующей неделе разберём воронку на IV квартал.

Пётр`) })
  M({ id: 'ar2', acct: A, folder: 'archive', from: 'hotel', to: [A], at: day(21, 12, 40), seen: true,
    subject: L('Booking confirmed: 3 nights, Pump Expo week', 'Бронирование подтверждено: 3 ночи, неделя выставки'),
    text: L(`Dear Ms Lindqvist,

Your booking at the Canal House Hotel is confirmed: 3 nights, superior single room, breakfast included. Booking reference CH-58214.

We look forward to welcoming you.`, `Уважаемая Анна!

Ваше бронирование в отеле «Канал» подтверждено: 3 ночи, одноместный номер улучшенной категории, завтрак включён. Номер брони CH-58214.

Будем рады вас видеть.`) })
  M({ id: 'ar3', acct: A, folder: 'archive', from: 'markus', to: [A], at: day(19, 10, 25), seen: true,
    subject: L('Alpenwerk catalogue 2027 and price update', 'Каталог Alpenwerk 2027 и изменение цен'),
    text: L(`Dear Anna,

Our new catalogue for 2027 is out. Prices for actuators go up by 3.5% from 1 January; orders placed before 15 December keep the current prices.

Best regards,
Markus Huber`, `Анна, добрый день!

Вышел наш каталог на 2027 год. Цены на приводы с 1 января вырастут на 3,5%; заказы, размещённые до 15 декабря, — по текущим ценам.

С уважением,
Маркус Хубер`) })
  // Spam and trash
  M({ id: 'sp1', acct: A, folder: 'junk', from: 'fakeAp', to: [A], at: day(2, 4, 13), seen: false, spam: true, av: ['macro'], auth: { spf: 'softfail', dkim: 'none', dmarc: 'fail' }, dmarc: 'fail',
    subject: L('OVERDUE: invoice 88231 — final notice', 'ПРОСРОЧЕНО: счёт 88231 — последнее напоминание'),
    text: L(`Dear customer,

Your invoice 88231 is 30 days overdue. Open the attached document and enable editing to see the payment details. Failure to pay within 24 hours will result in legal action.

Accounts Payable`, `Уважаемый клиент!

Счёт 88231 просрочен на 30 дней. Откройте вложенный документ и включите редактирование, чтобы увидеть реквизиты. Если оплата не поступит в течение 24 часов, мы обратимся в суд.

Отдел расчётов`),
    atts: [att('macro', 'Invoice_88231.docm')] })
  M({ id: 'sp2', acct: A, folder: 'junk', from: 'prize', to: [A], at: day(5, 3, 47), seen: true, spam: true,
    subject: L('Congratulations! You have been selected to receive a reward', 'Поздравляем! Вы выбраны получателем приза'),
    text: L(`You are our 1,000,000th visitor! Claim your reward now — the offer expires in 2 hours.`, `Вы наш миллионный посетитель! Заберите приз прямо сейчас — предложение действует 2 часа.`) })
  M({ id: 'tr1', acct: A, folder: 'trash', from: 'webinar', to: [A], at: day(15, 9, 0), seen: true,
    subject: L('Webinar: energy efficiency of pump systems', 'Вебинар: энергоэффективность насосных систем'),
    text: L(`Join our free webinar on Thursday at 15:00 — how variable speed drives cut energy costs by up to 40%.`, `Приглашаем на бесплатный вебинар в четверг в 15:00: как частотные приводы снижают затраты на электроэнергию до 40%.`) })
  // Own folder "Suppliers" (auto-sorted)
  M({ id: 'hg1', acct: A, folder: L('Suppliers', 'Поставщики'), from: A, to: ['li'], at: day(6, 8, 30), seen: true,
    subject: L('Sample castings HV-1186/1187 — photos please', 'Пробные отливки HV-1186/1187 — нужны фото'),
    text: L(`Dear Li Wei,

Before the sample castings HV-1186 and HV-1187 are shipped, could you send us photos of the flange faces and the bores?

Thank you,`, `Ли Вэй, добрый день!

Перед отправкой пробных отливок HV-1186 и HV-1187 пришлите, пожалуйста, фотографии фланцев и расточек.

Спасибо,`) })
  M({ id: 'hg2', acct: A, folder: L('Suppliers', 'Поставщики'), from: 'li', to: [A], at: day(4, 7, 55), seen: true, re: 'hg1', labels: [3], dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
    subject: L('Re: Sample castings HV-1186/1187 — photos please', 'Re: Пробные отливки HV-1186/1187 — нужны фото'),
    text: L(`Dear Anna,

Please see the photos attached. HV-1186 is fine. HV-1187 has a small surface crack near a bolt hole — we will cast it again at our cost and ship both together next week.

Best regards,
Li Wei
Export Sales, Ningbo Hengda Valve Co., Ltd.`, `Анна, добрый день!

Фото во вложении. HV-1186 в порядке. На HV-1187 у болтового отверстия небольшая поверхностная трещина — перельём за свой счёт и отправим обе отливки вместе на следующей неделе.

С уважением,
Ли Вэй
Экспортный отдел, Ningbo Hengda Valve Co., Ltd.`),
    atts: [att('casting1', 'HV-1186_flange.jpg'), att('casting2', 'HV-1187_crack.jpg')] })
  M({ id: 'br0', acct: A, folder: L('Suppliers', 'Поставщики'), from: 'jonas', to: [A], at: day(13, 11, 5), seen: true, dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
    subject: L('Seals for the Tidewater project — stock reserved', 'Уплотнения под проект верфи — зарезервировали'),
    text: L(`Hi Anna,

We have reserved 40 mechanical seals MS-45-SiC and 80 bearings 6312 C3 for your Tidewater project until the end of November.

Jonas`, `Анна, добрый день!

Зарезервировали под ваш проект для верфи 40 торцевых уплотнений MS-45-SiC и 80 подшипников 6312 C3 до конца ноября.

Юрий`) })

  // ===================== sales@ (shared) =====================
  M({ id: 'pl1', acct: S, folder: 'inbox', from: 'jonas', to: [S], at: day(9, 11, 40), seen: true, answered: true, dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
    subject: L('Updated price list Q4 2026', 'Обновлённый прайс-лист на IV квартал 2026'),
    text: L(`Dear colleagues,

Please find attached our price list for Q4 2026. Prices for seals are unchanged; bearings are 2–3% higher because of the steel price.

The prices are valid until 31 December.

Best regards,
Jonas Weber
Key Account Manager
Brightline Parts GmbH`, `Коллеги, добрый день!

Направляю прайс-лист на IV квартал 2026 года. Цены на уплотнения без изменений, подшипники подорожали на 2–3% из-за стали.

Цены действуют до 31 декабря.

С уважением,
Юрий Веберов
ведущий менеджер
ООО «Брайтлайн Партс»`),
    atts: [att('pricelist', L('Brightline_PriceList_Q4-2026.xlsx', 'Прайс_Брайтлайн_IV_кв_2026.xlsx'))] })
  M({ id: 'pl2', acct: S, folder: 'sent', from: S, by: 'mark', to: ['jonas'], at: day(9, 13, 2), seen: true, re: 'pl1',
    subject: L('Re: Updated price list Q4 2026', 'Re: Обновлённый прайс-лист на IV квартал 2026'),
    text: L(`Hello Jonas,

Thank you, received. Please keep the MS-45-SiC price for our Tidewater project — we expect the order in November.`, `Юрий, добрый день!

Спасибо, получили. Просим сохранить цену на MS-45-SiC под проект верфи — заказ ожидаем в ноябре.`) })
  M({ id: 'po1', acct: S, folder: 'inbox', from: 'daniel', to: [S], at: day(3, 16, 45), seen: true, answered: true, labels: [5],
    subject: L('PO 4471 — 6 electric valve actuators', 'Заказ № 4471 — 6 электроприводов'),
    text: L(`Hello,

Please find attached our purchase order 4471 for 6 NW-EA250 actuators with control units, as per your quote.

Kindly confirm the order and the delivery date.

Daniel Okafor
Procurement Officer
Maple Ridge Water Utility`, `Добрый день!

Направляем заказ № 4471 на 6 электроприводов NW-EA250 с блоками управления по вашему предложению.

Просим подтвердить заказ и срок поставки.

Сергей Гречко
начальник отдела снабжения
МУП «Водоканал Приозерска»`),
    atts: [att('po', L('PO-4471_Nordwave.pdf', 'Заказ_4471_Нордвейв.pdf'))] })
  M({ id: 'po2', acct: S, folder: 'sent', from: S, by: 'mark', to: ['daniel'], cc: ['oliver'], at: day(2, 10, 20), seen: true, re: 'po1',
    subject: L('Re: PO 4471 — 6 electric valve actuators', 'Re: Заказ № 4471 — 6 электроприводов'),
    text: L(`Dear Daniel,

Thank you for your order. We confirm PO 4471 in full. All 6 actuators are in stock and leave our warehouse tomorrow with Harbourside Logistics; delivery to Pump Station 3 the day after.

Oliver from Harbourside (in copy) will send you the tracking details.`, `Сергей, добрый день!

Спасибо за заказ. Подтверждаем заказ № 4471 полностью. Все 6 приводов на складе, завтра их забирает «Гавань Логистик», доставка на насосную станцию № 3 — послезавтра.

Олег из «Гавань Логистик» (в копии) пришлёт данные для отслеживания.`) })
  M({ id: 'po3', acct: S, folder: 'inbox', from: 'daniel', to: [S], at: ago(180), seen: false, re: 'po2', labels: [5],
    subject: L('Re: PO 4471 — 6 electric valve actuators', 'Re: Заказ № 4471 — 6 электроприводов'),
    text: L(`Hello Mark,

All 6 actuators arrived this morning in good condition. The signed delivery note is attached.

Could you send us the invoice and the commissioning certificates?

Thank you,
Daniel`, `Максим, добрый день!

Все 6 приводов пришли сегодня утром без повреждений. Подписанная накладная во вложении.

Пришлите, пожалуйста, счёт-фактуру и паспорта на приводы.

Спасибо,
Сергей`),
    atts: [att('scan', L('DN-77812_signed.pdf', 'Накладная_77812_подписана.pdf'))] })
  M({ id: 'al1', acct: S, folder: 'sent', from: S, by: 'anna', to: ['markus'], at: day(7, 9, 15), seen: true,
    subject: L('Delivery date for order AW-2291?', 'Срок поставки по заказу AW-2291?'),
    text: L(`Hello Markus,

Could you confirm the delivery date for our order AW-2291 (8 hydraulic actuators)? Our customer needs them on site by the end of November.`, `Маркус, добрый день!

Подтвердите, пожалуйста, срок поставки по нашему заказу AW-2291 (8 гидроприводов). Заказчику они нужны на объекте до конца ноября.`) })
  M({ id: 'al2', acct: S, folder: 'inbox', from: 'markus', to: [S], at: day(6, 14, 30), seen: true, re: 'al1',
    subject: L('Re: Delivery date for order AW-2291?', 'Re: Срок поставки по заказу AW-2291?'),
    text: L(`Hello Anna,

AW-2291 is in final testing. We ship in week 46 (10 November); transit to Bristol takes 4 days.

Best regards,
Markus Huber
Alpenwerk Hydraulik AG`, `Анна, добрый день!

AW-2291 на финальных испытаниях. Отгружаем на 46-й неделе (10 ноября), доставка до Санкт-Петербурга — 6 дней.

С уважением,
Маркус Хубер
Alpenwerk Hydraulik AG`) })
  M({ id: 'al3', acct: S, folder: 'sent', from: S, by: 'anna', to: ['markus'], at: day(6, 15, 5), seen: true, re: 'al2',
    subject: L('Re: Delivery date for order AW-2291?', 'Re: Срок поставки по заказу AW-2291?'),
    text: L(`Thank you, Markus — that works for us.`, `Спасибо, Маркус, нас это устраивает.`) })
  M({ id: 'lg1', acct: S, folder: 'inbox', from: 'oliver', to: [S], at: day(1, 15, 50), seen: false, labels: [5],
    subject: L('Pickup booked: Monday 09:00, 1 pallet to Rotterdam', 'Забор груза: понедельник 09:00, 1 паллета на верфь'),
    text: L(`Hello,

Your pickup is booked for Monday at 09:00: 1 pallet, 320 kg, Bristol → Tidewater Shipyards, Rotterdam. Booking HL-55107.

Please have the packing list ready for the driver.

Oliver Grant
Dispatch, Harbourside Logistics`, `Добрый день!

Забор груза назначен на понедельник, 09:00: 1 паллета, 320 кг, склад «Нордвейв» → Балтийская верфь. Заявка ГЛ-55107.

Подготовьте, пожалуйста, упаковочный лист для водителя.

Олег Гаврилов
диспетчер, ООО «Гавань Логистик»`) })
  M({ id: 'vs1', acct: S, folder: 'inbox', from: 'piotr', to: [S], at: day(4, 11, 10), seen: true, answered: true,
    subject: L('Partnership proposal: cast steel valve bodies', 'Предложение о сотрудничестве: стальные литые корпуса'),
    text: L(`Dear Sir or Madam,

Vistula Steel is a foundry in Warsaw producing cast steel bodies for valves and pumps (DN50–DN600, GP240GH and duplex). We would like to become one of your suppliers.

Can we send you samples and a price list?

Best regards,
Piotr Zieliński
Business Development, Vistula Steel`, `Добрый день!

«Минский литейный двор» производит стальные литые корпуса задвижек и насосов (DN50–DN600, сталь 25Л и дуплекс). Хотели бы стать вашим поставщиком.

Можем прислать образцы и прайс-лист?

С уважением,
Павел Зелинский
директор по развитию`) })
  M({ id: 'vs2', acct: S, folder: 'sent', from: S, by: 'anna', to: ['piotr'], at: day(3, 12, 0), seen: true, re: 'vs1',
    subject: L('Re: Partnership proposal: cast steel valve bodies', 'Re: Предложение о сотрудничестве: стальные литые корпуса'),
    text: L(`Dear Piotr,

Thank you for your proposal. We are interested in duplex bodies DN100–DN200 for seawater service.

Please send your price list and the material certificates (EN 10204 3.1). If the samples pass our inspection, we could place a trial order in Q1.`, `Павел, добрый день!

Спасибо за предложение. Нам интересны корпуса из дуплекса DN100–DN200 для морской воды.

Пришлите, пожалуйста, прайс-лист и сертификаты на материал (3.1 по EN 10204). Если образцы пройдут входной контроль, сможем разместить пробный заказ в I квартале.`) })
  M({ id: 'sv1', acct: S, folder: 'archive', from: 'dmitry', to: [S], at: day(12, 13, 30), seen: true,
    subject: L('Offer: gate valves DN150 PN16', 'Предложение: задвижки DN150 PN16'),
    html: 'gateValves' })
  M({ id: 'web1', acct: I, folder: 'inbox', from: 'visitor', to: [I], at: ago(60), seen: false,
    subject: L('Website enquiry: KP-300 in stock?', 'Вопрос с сайта: КП-300 в наличии?'),
    html: 'webForm' })

  // ===================== support@ (shared) =====================
  M({ id: 'vb1', acct: SU, folder: 'inbox', from: 'ruben', to: [SU], at: day(2, 8, 5), seen: true, answered: true, labels: [1],
    subject: L('URGENT: KP-200 vibrates after restart', 'СРОЧНО: КП-200 вибрирует после перезапуска'),
    text: L(`Hello support,

Since we restarted pump P3 (KP-200) after the motor replacement, it vibrates strongly and is noisy. We have switched it to the standby pump for now.

Nameplate photo attached.

Ruben Bakker
Maintenance Lead, Tidewater Shipyards
+31 6 5550 4471`, `Добрый день!

После замены двигателя и перезапуска насоса P3 (КП-200) сильная вибрация и шум. Пока перешли на резервный насос.

Фото шильдика во вложении.

Роман Беляев
главный механик, Балтийская Верфь Северо-Запад
+7 (921) 555-44-71`),
    atts: [att('nameplate', L('nameplate_P3.jpg', 'шильдик_P3.jpg'))] })
  M({ id: 'vb2', acct: SU, folder: 'sent', from: SU, by: 'sofia', to: ['ruben'], at: day(2, 9, 20), seen: true, re: 'vb1',
    subject: L('Re: URGENT: KP-200 vibrates after restart', 'Re: СРОЧНО: КП-200 вибрирует после перезапуска'),
    text: L(`Hello Ruben,

Thank you, ticket #2611-17 is open. To find the cause quickly, please send:
1. vibration readings at both bearings (mm/s), if you have a meter;
2. whether the coupling was re-aligned after the motor replacement;
3. the direction of rotation of the new motor.`, `Роман, добрый день!

Заявка № 2611-17 открыта. Чтобы быстро найти причину, пришлите, пожалуйста:
1. показания вибрации на обоих подшипниках (мм/с), если есть виброметр;
2. выполнялась ли центровка муфты после замены двигателя;
3. направление вращения нового двигателя.`) })
  M({ id: 'vb3', acct: SU, folder: 'inbox', from: 'ruben', to: [SU], at: day(1, 16, 40), seen: false, re: 'vb2', labels: [1],
    subject: L('Re: URGENT: KP-200 vibrates after restart', 'Re: СРОЧНО: КП-200 вибрирует после перезапуска'),
    text: L(`Hi Sofia,

1. Readings are in the attached log — up to 11 mm/s at the drive end.
2. No, the coupling was not re-aligned, the electrician only bolted the motor down.
3. Rotation is correct (checked against the arrow).

Can you come on Friday?

Ruben`, `София, добрый день!

1. Показания — в приложенном журнале, до 11 мм/с со стороны привода.
2. Нет, центровку не делали, электрик только закрепил двигатель.
3. Направление вращения правильное (сверили со стрелкой).

Сможете приехать в пятницу?

Роман`),
    atts: [att('csv', L('vibration_log_P3.csv', 'журнал_вибрации_P3.csv'))] })
  M({ id: 'sp10', acct: SU, folder: 'inbox', from: 'daniel', to: [SU], at: day(5, 13, 30), seen: true, answered: true,
    subject: L('Spare parts list for NW-EA250?', 'Перечень запчастей для NW-EA250?'),
    text: L(`Hello,

Could you send us the spare parts list for the NW-EA250 actuators? Our maintenance team wants to keep a small stock.

Daniel`, `Добрый день!

Пришлите, пожалуйста, перечень запасных частей для приводов NW-EA250 — служба эксплуатации хочет держать небольшой запас.

Сергей`) })
  M({ id: 'sp11', acct: SU, folder: 'sent', from: SU, by: 'sofia', to: ['daniel'], at: day(5, 15, 10), seen: true, re: 'sp10',
    subject: L('Re: Spare parts list for NW-EA250?', 'Re: Перечень запчастей для NW-EA250?'),
    text: L(`Hello Daniel,

Here is the recommended stock for 6 actuators:
• 2 × gearbox seal kit NW-EA-SK1
• 1 × limit switch module NW-EA-LS2
• 1 × motor capacitor NW-EA-C40

The full spare parts list is in section 9 of the manual. Our parts shop (shop@nordwave-parts.example) can deliver within 3 days.`, `Сергей, добрый день!

Рекомендуемый запас на 6 приводов:
• 2 × ремкомплект уплотнений редуктора NW-EA-SK1
• 1 × модуль концевых выключателей NW-EA-LS2
• 1 × конденсатор двигателя NW-EA-C40

Полный перечень — в разделе 9 руководства. Магазин запчастей (shop@nordwave-parts.example) доставит за 3 дня.`) })
  M({ id: 'sp12', acct: SU, folder: 'archive', from: 'ruben', to: [SU], at: day(27, 10, 0), seen: true,
    subject: L('Thank you — issue resolved', 'Спасибо — проблема решена'),
    text: L(`Hi Sofia, the new seal works perfectly, no leaks after two weeks. Thanks for the quick help! Ruben`, `София, новое уплотнение работает отлично, за две недели ни одной протечки. Спасибо за быструю помощь! Роман`) })

  // ===================== info@ (shared) =====================
  M({ id: 'inv1', acct: I, folder: L('Invoices', 'Счета'), from: 'brightAcc', to: [I], at: day(3, 9, 12), seen: false, dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
    subject: L('Invoice INV-2026-0418 — Nordwave Engineering & Trading', 'Счёт № 418 от 26.10.2026 — ООО «Нордвейв Инжиниринг»'),
    text: L(`Dear customer,

Please find attached our invoice INV-2026-0418 for your order NW-PO-2611. Amount due: EUR 12,016.00, payable by 25 November.

Brightline Parts GmbH — Accounts`, `Добрый день!

Во вложении счёт № 418 по вашему заказу НВ-2611. Сумма к оплате: 1 075 300,00 ₽, срок оплаты — до 25 ноября.

ООО «Брайтлайн Партс», бухгалтерия`),
    atts: [att('invoice', L('INV-2026-0418.pdf', 'Счёт_418_от_26.10.2026.pdf'))] })
  M({ id: 'inv0', acct: I, folder: L('Invoices', 'Счета'), from: 'oliver', to: [I], at: day(16, 14, 0), seen: true,
    subject: L('Invoice HL-2026-1187 — transport services October', 'Счёт ГЛ-1187 — транспортные услуги за октябрь'),
    text: L(`Hello, our invoice HL-2026-1187 for 6 deliveries in October is available in your customer portal. Amount: GBP 1,840.00, due in 14 days.`, `Добрый день! Счёт ГЛ-1187 за 6 доставок в октябре доступен в личном кабинете. Сумма 64 800 ₽, оплата в течение 14 дней.`) })
  M({ id: 'ex1', acct: I, folder: 'inbox', from: 'expo', to: [I], at: day(6, 9, 45), seen: true,
    subject: L('Pump Expo 2026 — exhibitor information and stand B-214', 'Насосы-Экспо 2026 — информация для участника, стенд B-214'),
    text: L(`Dear exhibitor,

Your stand B-214 (24 m²) is confirmed. Set-up starts at 08:00 the day before the opening. Exhibitor badges can be collected at the north entrance.

See you at Pump Expo!`, `Уважаемый участник!

Ваш стенд B-214 (24 м²) подтверждён. Монтаж — с 08:00 накануне открытия. Бейджи участников выдаются у северного входа.

До встречи на выставке!`) })
  M({ id: 'seo1', acct: I, folder: 'junk', from: 'seo', to: [I], at: day(3, 2, 18), seen: true, spam: true,
    subject: L('Your website is losing customers — first page of search in 7 days!', 'Ваш сайт теряет клиентов — первая страница поиска за 7 дней!'),
    text: L(`We guarantee top-3 positions for nordwave.example. Reply now for a free audit!`, `Гарантируем ТОП-3 для nordwave.example. Ответьте сейчас и получите бесплатный аудит!`) })

  // ===================== Gmail (collected) =====================
  M({ id: 'gm1', acct: G, folder: 'inbox', from: 'airline', to: [G], at: day(5, 19, 20), seen: true, dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
    subject: L('Your booking: Bristol → Amsterdam, flight SY 2041', 'Ваше бронирование: Санкт-Петербург → Москва, рейс SY 2041'),
    text: L(`Booking reference: QX7P2M

Bristol (BRS) → Amsterdam (AMS), flight SY 2041, departure 07:35, seat 12A.
Return: AMS → BRS, flight SY 2048, departure 19:10.

Online check-in opens 24 hours before departure.`, `Код бронирования: QX7P2M

Санкт-Петербург (LED) → Москва (SVO), рейс SY 2041, вылет 07:35, место 12A.
Обратно: SVO → LED, рейс SY 2048, вылет 19:10.

Онлайн-регистрация открывается за 24 часа до вылета.`) })
  M({ id: 'gm2', acct: G, folder: 'inbox', from: 'friend', to: [G], at: day(2, 20, 41), seen: true, answered: true,
    subject: L('Dinner on Saturday?', 'Ужин в субботу?'),
    text: L(`Hi Anna! We're finally back from Lisbon. Dinner at ours on Saturday, 7 pm? Bring Tom!

Emma`, `Анька, привет! Мы наконец вернулись из Лиссабона. Ужин у нас в субботу в 19:00? Бери Тимура!

Катя`) })
  M({ id: 'gm3', acct: G, folder: 'sent', from: G, to: ['friend'], at: day(1, 21, 10), seen: true, re: 'gm2',
    subject: L('Re: Dinner on Saturday?', 'Re: Ужин в субботу?'),
    text: L(`Yes, with pleasure! We'll bring dessert.`, `Да, с удовольствием! Мы с десертом.`) })
  M({ id: 'gm4', acct: G, folder: 'inbox', from: 'bank', to: [G], at: day(10, 6, 30), seen: true,
    subject: L('Your October statement is ready', 'Выписка за октябрь готова'),
    text: L(`Your account statement for October is available in online banking.`, `Выписка по счёту за октябрь доступна в интернет-банке.`) })
  M({ id: 'gm5', acct: G, folder: 'inbox', from: 'expo', to: [G], at: day(7, 12, 0), seen: true,
    subject: L('Your visitor badge for Pump Expo 2026', 'Ваш бейдж посетителя «Насосы-Экспо 2026»'),
    text: L(`Thank you for registering. Show this e-mail at the entrance to collect your badge. Badge number: PE26-118204.`, `Спасибо за регистрацию. Покажите это письмо на входе, чтобы получить бейдж. Номер бейджа: PE26-118204.`) })

  // ===================== letters filed in own folders =====================
  M({ id: 'tp1', acct: A, folder: L('Tidewater project', 'Проект верфи'), from: 'femke', to: [A], at: day(23, 11, 20), seen: true, labels: [2],
    subject: L('HT-40 tug programme — supplier pre-qualification', 'Буксиры проекта 22800 — предквалификация поставщиков'),
    text: L(`Dear Anna,

Tidewater is starting a series of 12 harbour tugs (HT-40). We would like to pre-qualify Nordwave as a supplier of seawater cooling pumps.

Please fill in the supplier questionnaire on our portal (reference SQ-2026-31) by the end of the month.

Kind regards,
Femke de Vries`, `Анна, добрый день!

Верфь начинает серию из 12 буксиров проекта 22800. Хотим включить «Нордвейв» в список квалифицированных поставщиков насосов забортной воды.

Анкета — на портале поставщиков, номер ПК-2026-31. Срок — до конца месяца.

С уважением,
Елена Фролова`) })
  M({ id: 'tp2', acct: A, folder: L('Tidewater project', 'Проект верфи'), from: A, to: ['femke'], at: day(22, 9, 40), seen: true, re: 'tp1',
    subject: L('Re: HT-40 tug programme — supplier pre-qualification', 'Re: Буксиры проекта 22800 — предквалификация поставщиков'),
    text: L(`Dear Femke,

Thank you — the questionnaire SQ-2026-31 is submitted. Our references for seawater pumps and the ISO 9001 certificate are uploaded too.`, `Елена, добрый день!

Спасибо, анкета ПК-2026-31 заполнена. Референции по насосам забортной воды и сертификат ISO 9001 тоже загрузили.`) })
  M({ id: 'or1', acct: S, folder: L('Orders', 'Заказы'), from: 'daniel', to: [S], at: day(18, 10, 5), seen: true,
    subject: L('PO 4388 — gearbox seal kits', 'Заказ № 4388 — ремкомплекты уплотнений'),
    text: L(`Hello, please supply 10 gearbox seal kits NW-EA-SK1 as per our framework agreement. PO 4388. Daniel`, `Добрый день! Просим поставить 10 ремкомплектов уплотнений NW-EA-SK1 по рамочному договору. Заказ № 4388. Сергей`) })
  M({ id: 'or2', acct: S, folder: L('Orders', 'Заказы'), from: S, by: 'mark', to: ['daniel'], at: day(18, 14, 30), seen: true, re: 'or1',
    subject: L('Re: PO 4388 — gearbox seal kits', 'Re: Заказ № 4388 — ремкомплекты уплотнений'),
    text: L(`Hello Daniel, PO 4388 is confirmed and shipped today, tracking HL-54980.`, `Сергей, добрый день! Заказ № 4388 подтверждён и сегодня отгружен, заявка перевозчика ГЛ-54980.`) })
  M({ id: 'wr1', acct: SU, folder: L('Warranty', 'Гарантия'), from: 'ruben', to: [SU], at: day(41, 8, 15), seen: true,
    subject: L('Warranty claim W-118: seal leak on KP-200 P1', 'Рекламация Г-118: течь уплотнения КП-200 P1'),
    text: L(`Hello, the mechanical seal of P1 leaks after 2,300 hours. Photos are in our portal, claim W-118. Ruben`, `Добрый день! Торцевое уплотнение насоса P1 потекло через 2 300 часов. Фото на портале, рекламация Г-118. Роман`) })
  M({ id: 'nl1', acct: I, folder: L('Newsletters', 'Рассылки'), from: 'webinar', to: [I], at: day(9, 9, 0), seen: true,
    subject: L('FlowTech Academy: November courses', 'Академия «ФлоуТех»: курсы ноября'),
    text: L(`New in November: pump selection for district heating, mechanical seals in practice, vibration diagnostics for beginners.`, `В ноябре: подбор насосов для теплосетей, торцевые уплотнения на практике, вибродиагностика для начинающих.`) })
  M({ id: 'nl2', acct: I, folder: L('Newsletters', 'Рассылки'), from: 'news', to: [I], at: day(8, 7, 0), seen: true,
    subject: L('Pump Industry Weekly #411: tariffs and lead times', 'Насосный вестник № 411: пошлины и сроки поставки'),
    text: L(`This week: what the new steel tariffs mean for pump prices, and why lead times are getting shorter again.`, `На этой неделе: как новые пошлины на сталь отразятся на ценах насосов и почему сроки поставки снова сокращаются.`) })
  M({ id: 'tv1', acct: G, folder: L('Travel', 'Поездки'), from: 'hotel', to: [G], at: day(20, 21, 5), seen: true,
    subject: L('Your stay at Canal House Hotel — check-in information', 'Ваше проживание в отеле «Канал» — информация о заезде'),
    text: L(`Check-in from 15:00, check-out until 11:00. The hotel is a 10-minute walk from the exhibition centre.`, `Заезд с 15:00, выезд до 11:00. До выставочного центра 10 минут пешком.`) })

  return out
}

// Letters that "arrive" when the demo is asked (POST /demo/push).
export function incoming(W, now) {
  const { L, D } = W
  const A = 'anna@' + D, S = 'sales@' + D, I = 'info@' + D
  return [
    { id: 'n1', acct: A, folder: 'inbox', from: 'jonas', to: [A], dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
      subject: L('Tidewater seals: price held until 30 November', 'Уплотнения для верфи: цену держим до 30 ноября'),
      text: L(`Hi Anna,

Good news: we can hold the current price for the 40 MS-45-SiC seals until 30 November, so your offer to Tidewater stays safe.

Jonas`, `Анна, добрый день!

Хорошая новость: цену на 40 уплотнений MS-45-SiC держим до 30 ноября, так что ваше предложение верфи в силе.

Юрий`) },
    { id: 'n2', acct: A, folder: 'inbox', from: 'femke', to: [A], re: 'tw5', labels: [2],
      subject: L('Re: Request for quotation: 40 seawater pumps for HT-40 tugs', 'Re: Запрос КП: 40 насосов забортной воды для буксиров'),
      text: L(`Anna, one more thing — could you also send a sample of the certificate of conformity? Our class surveyor wants to see the format.

Thanks,
Femke`, `Анна, и ещё: пришлите, пожалуйста, образец паспорта на насос — наш инспектор РМРС хочет посмотреть форму.

Спасибо,
Елена`) },
    { id: 'n3', acct: S, folder: 'inbox', from: 'oliver', to: [S], labels: [5],
      subject: L('Driver running 20 minutes late', 'Водитель опаздывает на 20 минут'),
      text: L(`Hello, the driver for booking HL-55107 is stuck in traffic and will be about 20 minutes late. Sorry for the inconvenience. Oliver`, `Добрый день! Водитель по заявке ГЛ-55107 стоит в пробке, опоздает минут на 20. Извините за неудобство. Олег`) },
    { id: 'n4', acct: I, folder: L('Invoices', 'Счета'), from: 'brightAcc', to: [I], filed: true, dmarc: 'pass', auth: { spf: 'pass', dkim: 'pass', dmarc: 'pass' },
      subject: L('Payment reminder: INV-2026-0418 due in 7 days', 'Напоминание: оплата счёта № 418 через 7 дней'),
      text: L(`Dear customer, this is a friendly reminder that invoice INV-2026-0418 (EUR 12,016.00) is due in 7 days.`, `Добрый день! Напоминаем, что срок оплаты счёта № 418 на 1 075 300,00 ₽ наступает через 7 дней.`) },
  ]
}
