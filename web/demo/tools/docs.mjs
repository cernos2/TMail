// Contents of the invented documents attached to the demo letters (both languages).
// Every company, address, number and bank here is fictional.

export const CSS = `
@page{size:A4;margin:0}
*{box-sizing:border-box}
html,body{margin:0;background:#fff}
body{font:10.5pt/1.45 "DejaVu Sans","Liberation Sans",Arial,sans-serif;color:#1d2530}
.page{width:210mm;min-height:297mm;padding:16mm 17mm 18mm;position:relative}
.top{display:flex;justify-content:space-between;align-items:flex-start;border-bottom:2px solid var(--c,#0a4ff5);padding-bottom:5mm;margin-bottom:7mm}
.brand{font:700 17pt/1.1 "DejaVu Sans",Arial,sans-serif;color:var(--c,#0a4ff5);letter-spacing:.02em}
.brand small{display:block;font:400 8.5pt/1.4 "DejaVu Sans",Arial,sans-serif;color:#5b6573;letter-spacing:0;margin-top:2mm}
.meta{text-align:right;font-size:9pt;color:#5b6573}
.meta b{display:block;font-size:15pt;color:#1d2530;margin-bottom:1mm}
h1{font-size:15pt;margin:0 0 4mm}
h2{font-size:11pt;margin:6mm 0 2mm;color:var(--c,#0a4ff5)}
.cols{display:flex;gap:10mm;margin-bottom:6mm}
.cols>div{flex:1;font-size:9.5pt}
.cols h3{font-size:8pt;text-transform:uppercase;letter-spacing:.06em;color:#8a94a3;margin:0 0 1.5mm}
table{width:100%;border-collapse:collapse;font-size:9.5pt;margin:2mm 0 4mm}
th{background:#eef2f7;text-align:left;padding:2.2mm 2.5mm;font-size:8.5pt;color:#3c4654;border-bottom:1px solid #cfd6e0}
td{padding:2.2mm 2.5mm;border-bottom:1px solid #e3e7ee;vertical-align:top}
td.n,th.n{text-align:right;white-space:nowrap}
.tot{margin-left:auto;width:78mm;font-size:10pt}
.tot div{display:flex;justify-content:space-between;padding:1.2mm 0}
.tot .big{font-weight:700;font-size:12pt;border-top:2px solid #1d2530;margin-top:1mm;padding-top:2mm}
.note{font-size:9pt;color:#3c4654;margin-top:6mm}
.foot{position:absolute;left:17mm;right:17mm;bottom:12mm;border-top:1px solid #dfe4ea;padding-top:3mm;font-size:7.5pt;color:#8a94a3;display:flex;justify-content:space-between;gap:6mm}
.sign{display:flex;justify-content:space-between;margin-top:14mm;font-size:9pt}
.sign div{width:70mm;border-top:1px solid #1d2530;padding-top:1.5mm}
p{margin:0 0 3mm}
ol{margin:0 0 3mm 4mm;padding-left:4mm}
li{margin-bottom:1.5mm}
`

const money = (n, cur, lang) => lang === 'ru'
  ? n.toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + (cur ? ' ' + cur : '')
  : (cur ? cur + ' ' : '') + n.toLocaleString('en-GB', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

function itemsTable(lang, items, cur, heads) {
  return `<table><thead><tr><th>#</th><th>${heads[0]}</th><th>${heads[1]}</th><th class="n">${heads[2]}</th><th class="n">${heads[3]}</th><th class="n">${heads[4]}</th></tr></thead><tbody>
${items.map((it, i) => `<tr><td>${i + 1}</td><td>${it[0]}</td><td>${it[1]}</td><td class="n">${it[2]}</td><td class="n">${money(it[3], '', lang)}</td><td class="n">${money(it[2] * it[3], '', lang)}</td></tr>`).join('')}
</tbody></table>`
}

export function documents(lang) {
  const ru = lang === 'ru'
  const docs = {}

  // ---------- supplier invoice ----------
  {
    const items = ru ? [
      ['Торцевое уплотнение 45 мм, SiC/SiC', 'MS-45-SiC', 40, 16800],
      ['Подшипник шариковый 6312 C3', 'BRG-6312-C3', 80, 3950],
      ['Комплект уплотнительных колец для КП-300, FKM', 'ORK-300', 40, 1720],
      ['Доставка до склада покупателя (Санкт-Петербург)', 'DLV', 1, 18500],
    ] : [
      ['Mechanical seal 45 mm, SiC/SiC', 'MS-45-SiC', 40, 186],
      ['Deep groove ball bearing 6312 C3', 'BRG-6312-C3', 80, 42.5],
      ['O-ring kit for KP-300, FKM', 'ORK-300', 40, 18.9],
      ['Freight DAP Bristol', 'FRT', 1, 420],
    ]
    const total = items.reduce((n, x) => n + x[2] * x[3], 0)
    docs.invoice = `<div class="page" style="--c:#c2410c">
<div class="top"><div class="brand">${ru ? 'ООО «Брайтлайн Партс»' : 'Brightline Parts GmbH'}<small>${ru ? '115280, г. Москва, ул. Автозаводская, д. 23, стр. 7<br>ИНН 7725461890 · КПП 772501001 · тел. +7 (495) 555-18-42' : 'Industriestraße 42 · 80939 München · Germany<br>USt-IdNr. DE287415963 · HRB 214587 · +49 89 5550 1842'}</small></div>
<div class="meta"><b>${ru ? 'Счёт № 418' : 'Invoice INV-2026-0418'}</b>${ru ? 'от 26 октября 2026 г.' : 'Date: 26 October 2026'}<br>${ru ? 'Основание: заказ № НВ-2611 от 12.10.2026' : 'Your order: NW-PO-2611 of 12 Oct 2026'}<br>${ru ? 'Оплатить до 25.11.2026' : 'Due date: 25 November 2026'}</div></div>
<div class="cols"><div><h3>${ru ? 'Покупатель' : 'Bill to'}</h3>${ru ? '<b>ООО «Нордвейв Инжиниринг»</b><br>199106, г. Санкт-Петербург, Средний пр. В.О., д. 88, офис 412<br>ИНН 7801654321 · КПП 780101001' : '<b>Nordwave Engineering &amp; Trading Ltd</b><br>14 Harbour Road, Bristol BS1 6XX<br>United Kingdom · VAT GB318447291'}</div>
<div><h3>${ru ? 'Банковские реквизиты' : 'Bank details'}</h3>${ru ? 'АО «Среднерусский Коммерческий Банк», г. Москва<br>БИК 044525999<br>р/с 40702810400000018427<br>к/с 30101810400000000999' : 'Isar Handelsbank AG, München<br>IBAN DE44 7009 9000 0012 3456 78<br>BIC ISHADEMMXXX'}</div></div>
${itemsTable(lang, items, ru ? '₽' : '€', ru ? ['Наименование', 'Артикул', 'Кол-во', 'Цена, ₽', 'Сумма, ₽'] : ['Description', 'Part no.', 'Qty', 'Unit, €', 'Amount, €'])
}
<div class="tot">${ru
  ? `<div><span>Итого:</span><span>${money(total, '₽', lang)}</span></div><div><span>В т. ч. НДС 22%:</span><span>${money(Math.round(total * 22 / 122 * 100) / 100, '₽', lang)}</span></div><div class="big"><span>Всего к оплате:</span><span>${money(total, '₽', lang)}</span></div>`
  : `<div><span>Subtotal</span><span>${money(total, '€', lang)}</span></div><div><span>VAT 0% (export outside the EU)</span><span>€ 0.00</span></div><div class="big"><span>Total due</span><span>${money(total, '€', lang)}</span></div>`}</div>
<p class="note">${ru ? 'Всего наименований 4, на сумму один миллион семьдесят пять тысяч триста рублей 00 копеек. Товар отгружается со склада в Подольске в течение 3 рабочих дней после оплаты.' : 'Payment terms: 30 days net. Please quote the invoice number with your payment. Goods are shipped from our Munich warehouse within 3 working days of the order confirmation.'}</p>
<div class="sign"><div>${ru ? 'Генеральный директор / Ю. А. Веберов' : 'Jonas Weber, Key Account Manager'}</div><div>${ru ? 'Главный бухгалтер / Т. С. Кремнёва' : 'Accounts receivable'}</div></div>
<div class="foot"><span>${ru ? 'ООО «Брайтлайн Партс» · brightline-parts.example' : 'Brightline Parts GmbH · Managing director: H. Brandl · brightline-parts.example'}</span><span>${ru ? 'Страница 1 из 1' : 'Page 1 of 1'}</span></div></div>`
  }

  // ---------- our commercial offer ----------
  {
    const items = ru ? [
      ['Насос центробежный КП-300/45, дуплекс 1.4462, двигатель 22 кВт', 'КП-300/45-D', 40, 612000],
      ['Шеф-монтаж и пусконаладка (выезд инженера, 5 дней)', 'SRV-COM', 1, 420000],
      ['Доставка до верфи (Санкт-Петербург, Кронштадт)', 'DLV', 1, 310000],
    ] : [
      ['Centrifugal pump KP-300/45, duplex 1.4462, 22 kW IE3 motor', 'KP-300/45-D', 40, 6850],
      ['Commissioning supervision (engineer on site, 5 days)', 'SRV-COM', 1, 4800],
      ['Delivery DAP Rotterdam, Tidewater yard', 'DLV', 1, 3900],
    ]
    const total = items.reduce((n, x) => n + x[2] * x[3], 0)
    docs.offer = `<div class="page">
<div class="top"><div class="brand">NORDWAVE<small>${ru ? 'ООО «Нордвейв Инжиниринг» · 199106, г. Санкт-Петербург, Средний пр. В.О., д. 88<br>ИНН 7801654321 · +7 (812) 555-01-42 · nordwave.example' : 'Nordwave Engineering &amp; Trading Ltd · 14 Harbour Road, Bristol BS1 6XX<br>Company no. 11847263 · +44 117 555 0142 · nordwave.example'}</small></div>
<div class="meta"><b>${ru ? 'Коммерческое предложение № 2611' : 'Commercial offer Q-2611'}</b>${ru ? 'от 28 октября 2026 г.' : '28 October 2026'}<br>${ru ? 'Действительно 30 дней' : 'Valid for 30 days'}</div></div>
<div class="cols"><div><h3>${ru ? 'Заказчик' : 'Customer'}</h3>${ru ? '<b>АО «Балтийская Верфь Северо-Запад»</b><br>Отдел закупок, Е. В. Фролова<br>198184, г. Санкт-Петербург, Канонерский о-в, д. 5' : '<b>Tidewater Shipyards B.V.</b><br>Procurement, Femke de Vries<br>Havenstraat 118, 3011 Rotterdam, Netherlands'}</div>
<div><h3>${ru ? 'Условия' : 'Terms'}</h3>${ru ? 'Срок поставки: 6–7 недель с даты предоплаты<br>Оплата: 50% предоплата, 50% по готовности<br>Гарантия: 24 месяца с даты пуска' : 'Lead time: 6–7 weeks from down payment<br>Payment: 50% with order, 50% before shipment<br>Warranty: 24 months from commissioning'}</div></div>
${itemsTable(lang, items.map(x => [x[0], x[1], x[2], x[3]]), '', ru ? ['Наименование', 'Код', 'Кол-во', 'Цена, ₽', 'Сумма, ₽'] : ['Description', 'Code', 'Qty', 'Unit, €', 'Amount, €'])}
<div class="tot"><div class="big"><span>${ru ? 'Итого с НДС 22%:' : 'Total (excl. VAT)'}</span><span>${money(total, ru ? '₽' : '€', lang)}</span></div></div>
<h2>${ru ? 'Технические данные насоса КП-300/45' : 'Pump data KP-300/45'}</h2>
<table><tbody>
<tr><td>${ru ? 'Подача / напор' : 'Flow / head'}</td><td>${ru ? '120 м³/ч при 45 м' : '120 m³/h at 45 m'}</td><td>${ru ? 'Перекачиваемая среда' : 'Medium'}</td><td>${ru ? 'морская вода, до 35 °C' : 'seawater, up to 35 °C'}</td></tr>
<tr><td>${ru ? 'Материал проточной части' : 'Wetted parts'}</td><td>${ru ? 'дуплекс 1.4462' : 'duplex 1.4462'}</td><td>${ru ? 'Уплотнение' : 'Shaft seal'}</td><td>${ru ? 'торцевое, SiC/SiC' : 'mechanical, SiC/SiC'}</td></tr>
<tr><td>${ru ? 'Двигатель' : 'Motor'}</td><td>22 kW IE3, 400 V, 50 Hz</td><td>${ru ? 'Классификация' : 'Class approval'}</td><td>${ru ? 'РМРС' : 'DNV'}</td></tr>
</tbody></table>
<p class="note">${ru ? 'При заказе всей партии одной поставкой возможна скидка — обсудим. Будем рады ответить на вопросы.' : 'A volume discount for a single shipment of the whole batch can be discussed. We will be glad to answer any questions.'}</p>
<div class="sign"><div>${ru ? 'Анна Соколова, руководитель отдела продаж' : 'Anna Lindqvist, Head of Sales'}</div><div></div></div>
<div class="foot"><span>Nordwave · nordwave.example</span><span>${ru ? 'Страница 1 из 1' : 'Page 1 of 1'}</span></div></div>`
  }

  // ---------- customer's requirements ----------
  docs.spec = `<div class="page" style="--c:#0f766e">
<div class="top"><div class="brand">${ru ? 'БАЛТИЙСКАЯ ВЕРФЬ СЕВЕРО-ЗАПАД' : 'TIDEWATER SHIPYARDS'}<small>${ru ? 'Технические требования к закупке' : 'Purchase specification'}</small></div>
<div class="meta"><b>${ru ? 'ТТ-2026-114' : 'SPEC-2026-114'}</b>${ru ? 'Проект: буксиры серии 22800' : 'Project: harbour tug series HT-40'}<br>Rev. B</div></div>
<h1>${ru ? 'Насосы забортной воды — требования' : 'Seawater cooling pumps — requirements'}</h1>
<table><thead><tr><th>${ru ? 'Параметр' : 'Parameter'}</th><th>${ru ? 'Требование' : 'Requirement'}</th></tr></thead><tbody>
${(ru ? [['Количество', '40 шт.'], ['Среда', 'забортная вода, 0…32 °C'], ['Подача', '120 м³/ч'], ['Напор', '45 м'], ['Материал', 'дуплекс 1.4462 или аналог'], ['Двигатель', '22 кВт, 380–400 В, 50 Гц, IP55'], ['Сертификация', 'РМРС'], ['Срок поставки', 'не позднее 50-й недели 2026 г.'], ['Место поставки', 'г. Санкт-Петербург, Канонерский о-в, д. 5']]
  : [['Quantity', '40 pcs'], ['Medium', 'seawater, 0…32 °C'], ['Flow', '120 m³/h'], ['Head', '45 m'], ['Material', 'duplex 1.4462 or equivalent'], ['Motor', '22 kW, 400 V, 50 Hz, IP55'], ['Class approval', 'DNV'], ['Delivery', 'week 50 / 2026 at the latest'], ['Place of delivery', 'Tidewater yard, Havenstraat 118, Rotterdam']]).map(r => `<tr><td>${r[0]}</td><td>${r[1]}</td></tr>`).join('')}
</tbody></table>
<p class="note">${ru ? 'Просим указать в предложении: срок поставки, гарантию, условия оплаты, наличие запасных частей на складе.' : 'Please state in your offer: lead time, warranty, payment terms and spare parts availability.'}</p>
<div class="foot"><span>${ru ? 'АО «Балтийская Верфь Северо-Запад»' : 'Tidewater Shipyards B.V.'}</span><span>${ru ? 'Страница 1 из 1' : 'Page 1 of 1'}</span></div></div>`

  // ---------- customer's purchase order ----------
  {
    const items = ru ? [
      ['Электропривод NW-EA250, 250 Н·м, IP68, 380 В', 'NW-EA250', 6, 184500],
      ['Блок управления с панелью оператора', 'NW-CU1', 6, 27300],
    ] : [
      ['Electric valve actuator NW-EA250, 250 Nm, IP68', 'NW-EA250', 6, 3180],
      ['Local control unit with operator panel', 'NW-CU1', 6, 470],
    ]
    const total = items.reduce((n, x) => n + x[2] * x[3], 0)
    docs.po = `<div class="page" style="--c:#1d4ed8">
<div class="top"><div class="brand">${ru ? 'МУП «ВОДОКАНАЛ ПРИОЗЕРСКА»' : 'MAPLE RIDGE WATER UTILITY'}<small>${ru ? '188760, Ленинградская обл., г. Приозерск, ул. Заводская, д. 9' : '2200 Ridge Road, Maple Ridge, BC V2X 0A1, Canada'}</small></div>
<div class="meta"><b>${ru ? 'Заказ № 4471' : 'Purchase order 4471'}</b>${ru ? 'от 26.10.2026' : '26 October 2026'}</div></div>
<div class="cols"><div><h3>${ru ? 'Поставщик' : 'Supplier'}</h3>${ru ? '<b>ООО «Нордвейв Инжиниринг»</b><br>г. Санкт-Петербург' : '<b>Nordwave Engineering &amp; Trading Ltd</b><br>Bristol, United Kingdom'}</div><div><h3>${ru ? 'Адрес доставки' : 'Ship to'}</h3>${ru ? 'Насосная станция № 3, г. Приозерск, ул. Береговая, д. 2' : 'Pump Station 3, 118 Lakeshore Dr, Maple Ridge, BC'}</div></div>
${itemsTable(lang, items, '', ru ? ['Наименование', 'Код', 'Кол-во', 'Цена, ₽', 'Сумма, ₽'] : ['Description', 'Code', 'Qty', 'Unit, CAD', 'Amount, CAD'])}
<div class="tot"><div class="big"><span>${ru ? 'Итого с НДС:' : 'Total, CAD'}</span><span>${money(total, '', lang)}</span></div></div>
<p class="note">${ru ? 'Оплата: 100% в течение 10 банковских дней после поставки. Просим подтвердить заказ и срок поставки.' : 'Terms: net 30 after delivery. Please confirm this order and the delivery date in writing.'}</p>
<div class="sign"><div>${ru ? 'Начальник отдела снабжения / С. А. Гречко' : 'Procurement — Daniel Okafor'}</div><div></div></div>
<div class="foot"><span>${ru ? 'МУП «Водоканал Приозерска»' : 'Maple Ridge Water Utility'}</span><span>1/1</span></div></div>`
  }

  // ---------- signed delivery note (scanned) ----------
  docs.scan = `<div class="page scan" style="--c:#334155">
<div class="top"><div class="brand">${ru ? 'ГАВАНЬ ЛОГИСТИК' : 'HARBOURSIDE LOGISTICS'}<small>${ru ? 'ООО «Гавань Логистик» · г. Санкт-Петербург, Митрофаньевское ш., д. 2' : 'Harbourside Logistics Ltd · Avonmouth Dock, Bristol BS11 9XX'}</small></div>
<div class="meta"><b>${ru ? 'Накладная № 77812' : 'Delivery note DN-77812'}</b>${ru ? 'от 27.10.2026' : '27 Oct 2026'}</div></div>
<div class="cols"><div><h3>${ru ? 'Отправитель' : 'Shipper'}</h3>${ru ? 'ООО «Нордвейв Инжиниринг»' : 'Nordwave Engineering &amp; Trading Ltd'}</div><div><h3>${ru ? 'Получатель' : 'Consignee'}</h3>${ru ? 'МУП «Водоканал Приозерска»' : 'Maple Ridge Water Utility'}</div></div>
<table><thead><tr><th>${ru ? 'Груз' : 'Goods'}</th><th class="n">${ru ? 'Мест' : 'Pallets'}</th><th class="n">${ru ? 'Вес, кг' : 'Weight, kg'}</th></tr></thead><tbody>
<tr><td>${ru ? 'Электроприводы NW-EA250' : 'Valve actuators NW-EA250'}</td><td class="n">2</td><td class="n">412</td></tr>
<tr><td>${ru ? 'Блоки управления NW-CU1' : 'Control units NW-CU1'}</td><td class="n">1</td><td class="n">96</td></tr>
</tbody></table>
<p>${ru ? 'Груз принят без повреждений, количество мест соответствует накладной.' : 'Goods received in good condition, number of pallets as stated.'}</p>
<div class="sign"><div>${ru ? 'Сдал: водитель А. Пеньков' : 'Driver: R. Hughes'}<br><span class="hand">${ru ? 'Пеньков' : 'R Hughes'}</span></div><div>${ru ? 'Принял: С. А. Гречко' : 'Received: D. Okafor'}<br><span class="hand">${ru ? 'Гречко' : 'D. Okafor'}</span></div></div>
<div class="stamp">${ru ? 'ПОЛУЧЕНО<br>28.10.2026' : 'RECEIVED<br>28 OCT 2026'}</div>
</div>`

  // ---------- distribution agreement (DOCX + its PDF preview) ----------
  docs.contractBlocks = ru ? [
    { title: 'ДИЛЕРСКИЙ ДОГОВОР № КПЕ-НВ/2026-07' },
    { p: 'г. Санкт-Петербург, «___» ноября 2026 г.' },
    { p: 'ТОО «Кестрел Пампс Евразия» (БИН 210940012345), именуемое в дальнейшем «Поставщик», в лице директора Ержана Нурланова, с одной стороны, и ООО «Нордвейв Инжиниринг» (ИНН 7801654321), именуемое в дальнейшем «Дилер», в лице генерального директора Петра Ковалёва, с другой стороны, заключили настоящий договор о нижеследующем.' },
    { h: '1. Предмет договора' },
    { p: '1.1. Поставщик предоставляет Дилеру право продажи насосного оборудования марки Kestrel на территории Северо-Западного федерального округа РФ.' },
    { p: '1.2. Дилер обязуется закупать оборудование в объёме не менее 18 000 000 рублей в год.' },
    { h: '2. Цены и порядок оплаты' },
    { p: '2.1. Цены определяются по прайс-листу Поставщика за вычетом дилерской скидки 22%.' },
    { p: '2.2. Оплата производится в течение 30 календарных дней с даты отгрузки.' },
    { h: '3. Поставка' },
    { p: '3.1. Поставка осуществляется на условиях FCA Алматы (Инкотермс 2020). Срок поставки — до 6 недель с даты заказа.' },
    { h: '4. Гарантия' },
    { p: '4.1. Гарантийный срок — 24 месяца с даты ввода в эксплуатацию, но не более 30 месяцев с даты отгрузки.' },
    { h: '5. Срок действия' },
    { p: '5.1. Договор действует два года с даты подписания и продлевается на каждый следующий год, если ни одна из сторон не заявит о его прекращении за 60 дней.' },
  ] : [
    { title: 'DISTRIBUTION AGREEMENT No. KPS-NW-2026/07' },
    { p: 'This Agreement is made on ___ November 2026 between Kestrel Pump Systems Inc., 4100 Gulf Freeway, Houston, TX 77023, USA (EIN 84-3920175), hereinafter "Supplier", and Nordwave Engineering & Trading Ltd, 14 Harbour Road, Bristol BS1 6XX, United Kingdom (Company no. 11847263), hereinafter "Distributor".' },
    { h: '1. Appointment' },
    { p: '1.1. The Supplier appoints the Distributor as its non-exclusive distributor of Kestrel centrifugal pumps and spare parts in the United Kingdom and Ireland.' },
    { p: '1.2. The Distributor shall purchase products for not less than USD 240,000 per contract year.' },
    { h: '2. Prices and payment' },
    { p: '2.1. Prices are the Supplier\'s list prices less a distributor discount of 22%.' },
    { p: '2.2. Invoices are payable within 30 days of the shipment date.' },
    { h: '3. Delivery' },
    { p: '3.1. Products are delivered FCA Houston (Incoterms 2020). Lead time is up to 6 weeks from the order.' },
    { h: '4. Warranty' },
    { p: '4.1. 24 months from commissioning, but no longer than 30 months from shipment.' },
    { h: '5. Term' },
    { p: '5.1. This Agreement runs for two years from signature and renews for further one-year periods unless terminated with 60 days\' notice.' },
  ]

  // ---------- translated manual (DOCX + PDF preview) ----------
  docs.manualBlocks = ru ? [
    { title: 'KP-300 Centrifugal Pump — Operating Manual' },
    { p: 'Translated from Russian. Version 3.2, October 2026.' },
    { h: '1. Safety instructions' },
    { p: 'Read this manual before installation and commissioning. Only qualified personnel may install, operate and maintain the pump. Disconnect the motor from the power supply before any work on the pump.' },
    { h: '2. Installation' },
    { p: 'Install the pump on a rigid, level foundation. Align the pump and motor shafts; the permissible radial misalignment is 0.05 mm. Support the suction and discharge pipes so that no forces act on the pump flanges.' },
    { h: '3. Commissioning' },
    { p: 'Fill the pump and the suction pipe with liquid. Check the direction of rotation (arrow on the bearing housing). Start the motor with the discharge valve slightly open, then open the valve slowly to the duty point.' },
    { h: '4. Maintenance' },
    { p: 'Check the mechanical seal for leaks weekly. Lubricate the bearings every 4,000 operating hours. Replace the O-rings at every seal replacement (kit ORK-300).' },
  ] : [
    { title: 'Betriebsanleitung Kreiselpumpe KP-300' },
    { p: 'Übersetzung aus dem Englischen. Version 3.2, Oktober 2026.' },
    { h: '1. Sicherheitshinweise' },
    { p: 'Lesen Sie diese Anleitung vor der Montage und Inbetriebnahme. Montage, Betrieb und Wartung dürfen nur von qualifiziertem Personal durchgeführt werden. Trennen Sie den Motor vor allen Arbeiten an der Pumpe vom Netz.' },
    { h: '2. Aufstellung' },
    { p: 'Die Pumpe auf einem steifen, ebenen Fundament aufstellen. Pumpen- und Motorwelle ausrichten; der zulässige Radialversatz beträgt 0,05 mm. Saug- und Druckleitung so abstützen, dass keine Kräfte auf die Pumpenflansche wirken.' },
    { h: '3. Inbetriebnahme' },
    { p: 'Pumpe und Saugleitung mit Flüssigkeit füllen. Drehrichtung prüfen (Pfeil am Lagergehäuse). Motor bei leicht geöffnetem Druckschieber starten und den Schieber langsam bis zum Betriebspunkt öffnen.' },
    { h: '4. Wartung' },
    { p: 'Gleitringdichtung wöchentlich auf Leckage prüfen. Lager alle 4.000 Betriebsstunden schmieren. O-Ringe bei jedem Dichtungswechsel ersetzen (Satz ORK-300).' },
  ]

  // ---------- supplier price list (XLSX) ----------
  docs.sheets = ru ? [
    { name: 'Уплотнения', widths: [14, 46, 8, 14, 12, 14], rows: [
      ['Артикул', 'Наименование', 'Ед.', 'Цена, ₽', 'Мин. партия', 'Срок, дней'],
      ['MS-35-SiC', 'Торцевое уплотнение 35 мм, SiC/SiC', 'шт', 14200, 10, 5],
      ['MS-45-SiC', 'Торцевое уплотнение 45 мм, SiC/SiC', 'шт', 16800, 10, 5],
      ['MS-55-TC', 'Торцевое уплотнение 55 мм, карбид вольфрама', 'шт', 21900, 5, 14],
      ['ORK-200', 'Комплект колец для КП-200, EPDM', 'компл', 1380, 20, 3],
      ['ORK-300', 'Комплект колец для КП-300, FKM', 'компл', 1720, 20, 3],
      ['GS-150', 'Сальниковая набивка 12 мм, PTFE, 5 м', 'рулон', 4650, 5, 7],
    ] },
    { name: 'Подшипники', widths: [14, 46, 8, 14, 12, 14], rows: [
      ['Артикул', 'Наименование', 'Ед.', 'Цена, ₽', 'Мин. партия', 'Срок, дней'],
      ['BRG-6308-C3', 'Подшипник шариковый 6308 C3', 'шт', 1650, 20, 3],
      ['BRG-6310-C3', 'Подшипник шариковый 6310 C3', 'шт', 2480, 20, 3],
      ['BRG-6312-C3', 'Подшипник шариковый 6312 C3', 'шт', 3950, 20, 3],
      ['BRG-7312-B', 'Радиально-упорный подшипник 7312 B', 'шт', 7840, 10, 10],
    ] },
  ] : [
    { name: 'Seals', widths: [14, 46, 8, 12, 10, 14], rows: [
      ['Part no.', 'Description', 'Unit', 'Price, €', 'MOQ', 'Lead time, days'],
      ['MS-35-SiC', 'Mechanical seal 35 mm, SiC/SiC', 'pc', 158, 10, 5],
      ['MS-45-SiC', 'Mechanical seal 45 mm, SiC/SiC', 'pc', 186, 10, 5],
      ['MS-55-TC', 'Mechanical seal 55 mm, tungsten carbide', 'pc', 243.5, 5, 14],
      ['ORK-200', 'O-ring kit for KP-200, EPDM', 'kit', 15.4, 20, 3],
      ['ORK-300', 'O-ring kit for KP-300, FKM', 'kit', 18.9, 20, 3],
      ['GS-150', 'Gland packing 12 mm, PTFE, 5 m', 'roll', 51.7, 5, 7],
    ] },
    { name: 'Bearings', widths: [14, 46, 8, 12, 10, 14], rows: [
      ['Part no.', 'Description', 'Unit', 'Price, €', 'MOQ', 'Lead time, days'],
      ['BRG-6308-C3', 'Deep groove ball bearing 6308 C3', 'pc', 18.3, 20, 3],
      ['BRG-6310-C3', 'Deep groove ball bearing 6310 C3', 'pc', 27.6, 20, 3],
      ['BRG-6312-C3', 'Deep groove ball bearing 6312 C3', 'pc', 42.5, 20, 3],
      ['BRG-7312-B', 'Angular contact bearing 7312 B', 'pc', 87.1, 10, 10],
    ] },
  ]
  // A fuller price list: more sizes of the same families.
  for (const sh of docs.sheets) {
    const seals = sh === docs.sheets[0]
    const extra = []
    for (const [i, d] of [25, 30, 40, 50, 60, 65, 70, 80, 90, 100].entries()) {
      extra.push(seals
        ? [`MS-${d}-C`, ru ? `Торцевое уплотнение ${d} мм, углерод/SiC` : `Mechanical seal ${d} mm, carbon/SiC`, ru ? 'шт' : 'pc', ru ? 9800 + d * 210 : 96 + d * 2.4, 10, 5 + (i % 3) * 4]
        : [`BRG-62${String(d / 5 | 0).padStart(2, '0')}-2RS`, ru ? `Подшипник закрытый 62${String(d / 5 | 0).padStart(2, '0')}-2RS` : `Sealed ball bearing 62${String(d / 5 | 0).padStart(2, '0')}-2RS`, ru ? 'шт' : 'pc', ru ? 640 + d * 38 : 6.8 + d * 0.42, 20, 3 + (i % 2) * 4])
    }
    sh.rows.push(...extra)
  }
  return docs
}

// A DOCX as a printable page (for the "office" preview of the attachment).
export const blocksHtml = blocks => `<div class="page">${blocks.map(b => b.title ? `<h1 style="text-align:center;margin-bottom:6mm">${b.title}</h1>`
  : b.h ? `<h2 style="color:#1d2530">${b.h}</h2>` : `<p style="text-align:justify">${b.p}</p>`).join('')}</div>`

// The spreadsheet as LibreOffice would convert it: an <h1> name, then a table, per sheet.
export const sheetsHtml = (sheets, lang) => `<!doctype html><html><head><meta charset="utf-8"></head><body>${sheets.map((s, i) =>
  `<h1>${lang === 'ru' ? 'Лист' : 'Sheet'} ${i + 1}: ${s.name}</h1><table border="0" cellspacing="0">${s.rows.map((r, ri) => `<tr>${r.map(v =>
    `<td${typeof v === 'number' ? ' align="right"' : ''}${ri === 0 ? ' bgcolor="#DDEBF7"' : ''}>${ri === 0 ? `<b>${v}</b>` : typeof v === 'number' && !Number.isInteger(v) ? v.toFixed(2) : v}</td>`).join('')}</tr>`).join('')}</table>`).join('')}</body></html>`
