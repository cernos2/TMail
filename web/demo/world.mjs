// The invented company of the demo, its people, mailboxes and counterparties (English and Russian).
// All domains use the reserved .example TLD; every name, number and address is fictional.

export function world(lang) {
  const ru = lang === 'ru'
  const L = (en, r) => (ru ? r : en)
  const D = 'nordwave.example'
  const D2 = 'nordwave-parts.example'

  const org = {
    name: L('Nordwave Engineering & Trading', 'Нордвейв Инжиниринг'),
    legal: L('Nordwave Engineering & Trading Ltd', 'ООО «Нордвейв Инжиниринг»'),
    country: L('GB', 'RU'),
    host: 'mail.' + D,
    phone: L('+44 117 555 0142', '+7 (812) 555-01-42'),
    lang, timezone: L('Europe/London', 'Europe/Moscow'),
  }

  // Our people (webmail users). login → user.
  const users = [
    { id: 1, login: 'anna', name: L('Anna Lindqvist', 'Анна Соколова'), title: L('Head of Sales', 'Руководитель отдела продаж'), isAdmin: true, personal: 'anna@' + D },
    { id: 2, login: L('mark', 'maxim'), name: L('Mark Ellison', 'Максим Орлов'), title: L('Sales Manager', 'Менеджер по продажам'), personal: L('mark@', 'maxim@') + D },
    { id: 3, login: 'sofia', name: L('Sofia Brandt', 'София Белова'), title: L('Service Engineer', 'Сервисный инженер'), personal: 'sofia@' + D },
    { id: 4, login: L('peter', 'petr'), name: L('Peter Novak', 'Пётр Ковалёв'), title: L('Managing Director', 'Генеральный директор'), isAdmin: true, personal: L('peter@', 'petr@') + D },
    { id: 5, login: L('lena', 'elena'), name: L('Lena Hoffmann', 'Елена Морозова'), title: L('Chief Accountant', 'Главный бухгалтер'), personal: L('lena@', 'elena@') + D },
    { id: 6, login: L('tom', 'timur'), name: L('Tom Reyes', 'Тимур Рахимов'), title: L('Logistics Coordinator', 'Логист'), personal: '', active: false },
  ]
  const U = Object.fromEntries(users.map(u => [u.id === 1 ? 'anna' : u.id === 2 ? 'mark' : u.id === 3 ? 'sofia' : u.id === 4 ? 'peter' : u.id === 5 ? 'lena' : 'tom', u]))

  const gmail = L('anna.lindqvist@gmail.example', 'anna.sokolova@gmail.example')
  // Mailboxes. users: [userId, canSend, personal]
  const mailboxes = [
    { id: 1, address: 'anna@' + D, name: U.anna.name, quotaGB: 10, usedMB: 3870, messages: 6412, users: [[1, true, true]] },
    { id: 2, address: U.mark.personal, name: U.mark.name, quotaGB: 10, usedMB: 2214, messages: 3980, users: [[2, true, true]] },
    { id: 3, address: 'sofia@' + D, name: U.sofia.name, quotaGB: 5, usedMB: 1120, messages: 2153, users: [[3, true, true]] },
    { id: 4, address: U.peter.personal, name: U.peter.name, quotaGB: 20, usedMB: 9640, messages: 15873, users: [[4, true, true]] },
    { id: 5, address: U.lena.personal, name: U.lena.name, quotaGB: 5, usedMB: 2930, messages: 4721, users: [[5, true, true]] },
    { id: 6, address: 'sales@' + D, name: L('Nordwave Sales', 'Нордвейв — отдел продаж'), quotaGB: 20, usedMB: 12870, messages: 21544, users: [[1, true, false], [2, true, false], [4, false, false], [6, false, false]] },
    { id: 7, address: 'support@' + D, name: L('Nordwave Service', 'Нордвейв — сервис'), quotaGB: 10, usedMB: 4580, messages: 7310, users: [[1, true, false], [3, true, false]] },
    { id: 8, address: 'info@' + D, name: L('Nordwave', 'Нордвейв'), quotaGB: 10, usedMB: 5920, messages: 11962, users: [[1, true, false], [2, true, false], [4, true, false], [5, false, false]] },
    { id: 9, address: 'shop@' + D2, name: L('Nordwave Parts Shop', 'Магазин запчастей Нордвейв'), quotaGB: 5, usedMB: 610, messages: 1844, users: [[2, true, false], [6, true, false]] },
    { id: 10, address: gmail, name: L('Anna Lindqvist (Gmail)', 'Анна Соколова (Gmail)'), quotaGB: 0, usedMB: 5530, messages: 9381, users: [[1, true, false]], external: true },
  ]
  // What Anna (the signed-in administrator) works with, in her order.
  const annaAccounts = ['anna@' + D, 'sales@' + D, 'support@' + D, 'info@' + D, gmail]

  // Signatures: Anna's own one has the company logo ({{ORIGIN}} is the demo server's address).
  const sigAnna = `<div><b>${U.anna.name}</b><br>${U.anna.title} · ${org.name}<br>${org.phone} · <a href="https://nordwave.example">nordwave.example</a></div><div><img src="{{ORIGIN}}/i/nordwave-logo.png" alt="Nordwave" width="130" height="28"></div>`
  const sigShared = (dept, phone) => `{name}\n${dept}\n${phone} · nordwave.example`
  const signatures = {
    ['anna@' + D]: sigAnna,
    ['sales@' + D]: sigShared(L('Nordwave Sales', 'Нордвейв — отдел продаж'), L('+44 117 555 0140', '+7 (812) 555-01-40')),
    ['support@' + D]: sigShared(L('Nordwave Service · 24/7 hotline', 'Сервисная служба Нордвейв · круглосуточно'), L('+44 117 555 0199', '+7 (812) 555-01-99')),
    ['info@' + D]: sigShared(org.name, org.phone),
    [gmail]: L('Anna', 'Анна'),
  }
  const startTemplates = {
    ['anna@' + D]: '',
    ['sales@' + D]: L('Hello,', 'Добрый день!'),
    ['support@' + D]: L('Hello,', 'Здравствуйте!'),
    ['info@' + D]: L('Hello,', 'Добрый день!'),
    [gmail]: '',
  }

  // People outside (correspondents). key → { name, email, company, position, phone }
  const P = {
    femke: { name: L('Femke de Vries', 'Елена Фролова'), email: L('f.devries@tidewater-shipyards.example', 'e.frolova@bsz-shipyard.example'), co: 'tide', position: L('Procurement Manager', 'Начальник отдела закупок'), phone: L('+31 10 555 2318', '+7 (812) 555-23-18') },
    ruben: { name: L('Ruben Bakker', 'Роман Беляев'), email: L('r.bakker@tidewater-shipyards.example', 'r.belyaev@bsz-shipyard.example'), co: 'tide', position: L('Maintenance Lead', 'Главный механик'), phone: L('+31 6 5550 4471', '+7 (921) 555-44-71') },
    jonas: { name: L('Jonas Weber', 'Юрий Веберов'), email: L('j.weber@brightline-parts.example', 'y.weberov@brightline-parts.example'), co: 'bright', position: L('Key Account Manager', 'Ведущий менеджер'), phone: L('+49 89 5550 1842', '+7 (495) 555-18-42') },
    brightAcc: { name: L('Brightline Accounts', 'Бухгалтерия «Брайтлайн Партс»'), email: L('invoices@brightline-parts.example', 'buh@brightline-parts.example'), co: 'bright' },
    rachel: { name: L('Rachel Kim', 'Ержан Нурланов'), email: L('rachel.kim@kestrel-pumps.example', 'y.nurlanov@kestrel-eurasia.example'), co: 'kestrel', position: L('Channel Manager EMEA', 'Директор'), phone: L('+1 713 555 0187', '+7 (727) 355-01-87') },
    oliver: { name: L('Oliver Grant', 'Олег Гаврилов'), email: L('o.grant@harbourside-logistics.example', 'o.gavrilov@gavan-logistic.example'), co: 'harbour', position: L('Dispatch', 'Диспетчер'), phone: L('+44 117 555 0388', '+7 (812) 555-03-88') },
    markus: { name: L('Markus Huber', 'Маркус Хубер'), email: 'm.huber@alpenwerk.example', co: 'alpen', position: L('Sales, Northern Europe', 'Менеджер по России и СНГ'), phone: '+41 44 555 72 10' },
    piotr: { name: L('Piotr Zieliński', 'Павел Зелинский'), email: L('p.zielinski@vistula-steel.example', 'p.zelinsky@minsk-foundry.example'), co: 'vistula', position: L('Business Development', 'Директор по развитию'), phone: L('+48 22 555 81 40', '+375 17 555-81-40') },
    daniel: { name: L('Daniel Okafor', 'Сергей Гречко'), email: L('d.okafor@mapleridge-water.example', 's.grechko@vodokanal-priozersk.example'), co: 'maple', position: L('Procurement Officer', 'Начальник отдела снабжения'), phone: L('+1 604 555 0144', '+7 (813) 795-55-44') },
    li: { name: L('Li Wei', 'Ли Вэй'), email: 'liwei@hengda-valve.example', co: 'hengda', position: L('Export Sales', 'Экспортный отдел'), phone: '+86 574 5555 8821' },
    dmitry: { name: L('Dmitry Volkov', 'Дмитрий Волков'), email: 'd.volkov@sevarm.example', co: 'sevarm', position: L('Sales Director', 'Коммерческий директор'), phone: '+7 (812) 555-77-30' },
    clara: { name: L('Clara Mendes', 'Ирина Лаптева'), email: L('clara.mendes@gmail.example', 'irina.lapteva@gmail.example'), position: L('Technical translator', 'Технический переводчик'), phone: L('+44 7700 555 214', '+7 (911) 555-22-14') },
    news: { name: L('Pump Industry Weekly', 'Насосный вестник'), email: 'news@pumpweekly.example' },
    phish: { name: L('Nordwave IT Helpdesk', 'ИТ-поддержка Нордвейв'), email: 'it-helpdesk@nordwave-secure.example' },
    prize: { name: L('Global Rewards Centre', 'Центр призов'), email: 'winner@prize-centre.example' },
    fakeAp: { name: L('Accounts Payable', 'Отдел расчётов'), email: 'ap@fastpay-invoices.example' },
    seo: { name: L('Top Ranking Agency', 'Агентство «Топ-Выдача»'), email: 'offer@rank-boost.example' },
    visitor: { name: L('John Carter', 'Иван Карпов'), email: L('john.carter@mailbox.example', 'i.karpov@mailbox.example') },
    airline: { name: L('Skyway Air', 'Скайвэй Эйр'), email: 'booking@skyway-air.example' },
    friend: { name: L('Emma Hayes', 'Катя Лебедева'), email: L('emma.hayes@gmail.example', 'katya.lebedeva@gmail.example') },
    expo: { name: L('Pump Expo 2026', 'Насосы-Экспо 2026'), email: 'badges@pumpexpo.example' },
    bank: { name: L('Harbour Bank', 'Банк «Гавань»'), email: 'statements@harbourbank.example' },
    hotel: { name: L('Canal House Hotel', 'Отель «Канал»'), email: 'reservations@canalhouse-hotel.example' },
    webinar: { name: L('FlowTech Academy', 'Академия «ФлоуТех»'), email: 'events@flowtech-academy.example' },
  }
  // Our own people as correspondents.
  for (const [k, u] of Object.entries(U)) P[k] = { name: u.name, email: u.personal || (k === 'tom' ? L('tom@', 'timur@') + D : ''), own: true, position: u.title }

  // Counterparty companies (CRM). Countries and identifiers differ by dataset.
  const C = {
    own: { name: org.legal, country: org.country, own: true, domains: [D, D2],
      inn: L('11847263', '7801654321'), ids: L({ VAT: 'GB318447291' }, {}), kpp: L('', '780101001'), ogrn: L('', '1187847123456'),
      legalAddress: L('14 Harbour Road, Bristol BS1 6XX, United Kingdom', '199106, г. Санкт-Петербург, Средний пр. В.О., д. 88, офис 412'),
      bankName: L('Westbridge Bank plc', 'ПАО «Балтийский Расчётный Банк»'), bik: L('20-41-77', '044030999'), account: L('GB29 WBBK 2041 7753 9201 84', '40702810900001234567'), corrAccount: L('', '30101810900000000999'),
      website: 'nordwave.example', phones: [org.phone], emails: ['info@' + D] },
    tide: { name: L('Tidewater Shipyards B.V.', 'АО «Балтийская Верфь Северо-Запад»'), country: L('NL', 'RU'), domains: [L('tidewater-shipyards.example', 'bsz-shipyard.example')],
      inn: L('58234117', '7805123459'), ids: L({ 'BTW (VAT)': 'NL858234117B01' }, {}), kpp: L('', '780501001'), ogrn: L('', '1027802712345'),
      legalAddress: L('Havenstraat 118, 3011 Rotterdam, Netherlands', '198184, г. Санкт-Петербург, Канонерский о-в, д. 5'),
      bankName: L('Maasbank N.V.', 'АО «Невский Промышленный Банк»'), bik: L('MAASNL2R', '044030888'), account: L('NL91 MAAS 0417 1643 00', '40702810300000045678'), corrAccount: L('', '30101810100000000888'),
      website: L('tidewater-shipyards.example', 'bsz-shipyard.example'), phones: [L('+31 10 555 2300', '+7 (812) 555-23-00')], notes: L('Shipyard, builds harbour tugs (series HT-40). Tender for 40 seawater pumps, decision expected in November.', 'Верфь, строит буксиры серии 22800. Тендер на 40 насосов забортной воды, решение ожидается в ноябре.') },
    bright: { name: L('Brightline Parts GmbH', 'ООО «Брайтлайн Партс»'), country: L('DE', 'RU'), domains: ['brightline-parts.example'],
      inn: L('DE287415963', '7725461890'), ids: L({ HRB: 'München HRB 214587' }, {}), kpp: L('', '772501001'), ogrn: L('', '1157746123987'),
      legalAddress: L('Industriestraße 42, 80939 München, Germany', '115280, г. Москва, ул. Автозаводская, д. 23, стр. 7'),
      bankName: L('Isar Handelsbank AG', 'АО «Среднерусский Коммерческий Банк»'), bik: L('ISHADEMMXXX', '044525999'), account: L('DE44 7009 9000 0012 3456 78', '40702810400000018427'), corrAccount: L('', '30101810400000000999'),
      website: 'brightline-parts.example', phones: [L('+49 89 5550 1800', '+7 (495) 555-18-00')], notes: L('Main supplier of mechanical seals, bearings and O-ring kits. Payment 30 days net.', 'Основной поставщик торцевых уплотнений, подшипников и комплектов колец. Отсрочка 30 дней.') },
    kestrel: { name: L('Kestrel Pump Systems Inc.', 'ТОО «Кестрел Пампс Евразия»'), country: L('US', 'KZ'), domains: [L('kestrel-pumps.example', 'kestrel-eurasia.example')],
      inn: L('84-3920175', '210940012345'), ids: L({ 'DUNS': '08-447-2913' }, { 'НДС (KZ)': '600400123456' }),
      legalAddress: L('4100 Gulf Freeway, Houston, TX 77023, USA', 'Казахстан, 050040, г. Алматы, пр. Аль-Фараби, д. 77/8'),
      bankName: L('Lone Star Commerce Bank', 'АО «Степной Банк»'), bik: L('111 000 614', 'STEPKZKA'), account: L('2049 1183 7720', 'KZ86 125K ZT50 0410 0100'),
      website: L('kestrel-pumps.example', 'kestrel-eurasia.example'), phones: [L('+1 713 555 0100', '+7 (727) 355-01-00')], notes: L('Pump manufacturer. Negotiating a distribution agreement for the UK and Ireland.', 'Производитель насосов. Обсуждаем дилерский договор на Северо-Запад.') },
    harbour: { name: L('Harbourside Logistics Ltd', 'ООО «Гавань Логистик»'), country: L('GB', 'RU'), domains: [L('harbourside-logistics.example', 'gavan-logistic.example')],
      inn: L('09876543', '7810456782'), ids: L({ VAT: 'GB 412 7781 06' }, {}), kpp: L('', '781001001'), ogrn: L('', '1167847334455'),
      legalAddress: L('Avonmouth Dock, Bristol BS11 9XX, United Kingdom', '196084, г. Санкт-Петербург, Митрофаньевское ш., д. 2'),
      website: L('harbourside-logistics.example', 'gavan-logistic.example'), phones: [L('+44 117 555 0300', '+7 (812) 555-03-00')], notes: L('Our forwarder for UK and EU deliveries.', 'Наш перевозчик по Санкт-Петербургу и области.') },
    alpen: { name: 'Alpenwerk Hydraulik AG', country: 'CH', domains: ['alpenwerk.example'],
      inn: 'CHE-412.736.905', ids: { MWST: 'CHE-412.736.905 MWST' }, legalAddress: 'Seestrasse 210, 8810 Horgen, Switzerland',
      bankName: L('Zürichsee Kantonalbank', 'Zürichsee Kantonalbank'), bik: 'ZSKBCH22', account: 'CH93 0076 2011 6238 5295 7',
      website: 'alpenwerk.example', phones: ['+41 44 555 72 00'], notes: L('Supplier of hydraulic valve actuators and impellers.', 'Поставщик гидроприводов и рабочих колёс.') },
    vistula: { name: L('Vistula Steel Sp. z o.o.', 'СП ООО «Минский литейный двор»'), country: L('PL', 'BY'), domains: [L('vistula-steel.example', 'minsk-foundry.example')],
      inn: L('5272918346', '190123456'), ids: L({ KRS: '0000481936', REGON: '146913720' }, {}),
      legalAddress: L('ul. Przemysłowa 14, 03-218 Warszawa, Poland', 'Беларусь, 220075, г. Минск, ул. Промышленная, д. 14'),
      website: L('vistula-steel.example', 'minsk-foundry.example'), phones: [L('+48 22 555 81 00', '+375 17 555-81-00')] },
    maple: { name: L('Maple Ridge Water Utility', 'МУП «Водоканал Приозерска»'), country: L('CA', 'RU'), domains: [L('mapleridge-water.example', 'vodokanal-priozersk.example')],
      inn: L('861450297', '4712001234'), ids: L({ 'GST/HST': '861450297 RT0001' }, {}), kpp: L('', '471201001'), ogrn: L('', '1024701645678'),
      legalAddress: L('2200 Ridge Road, Maple Ridge, BC V2X 0A1, Canada', '188760, Ленинградская обл., г. Приозерск, ул. Заводская, д. 9'),
      website: L('mapleridge-water.example', 'vodokanal-priozersk.example'), phones: [L('+1 604 555 0100', '+7 (813) 795-55-00')], notes: L('Municipal customer: valve actuators for pump stations. Pays on time.', 'Муниципальный заказчик: электроприводы для насосных станций. Платит в срок.') },
    hengda: { name: 'Ningbo Hengda Valve Co., Ltd.', country: 'CN', domains: ['hengda-valve.example'],
      inn: '91330206MA2AH4K97X', ids: {}, legalAddress: L('No. 88 Xinggang Road, Beilun District, Ningbo, China', 'Китай, г. Нинбо, район Бэйлунь, ул. Синган, 88'),
      website: 'hengda-valve.example', phones: ['+86 574 5555 8800'], notes: L('Foundry: valve body castings. Sample batch HV-1186/1187 under inspection.', 'Литейное производство: корпуса задвижек. Пробная партия HV-1186/1187 на проверке.') },
    sevarm: { name: L('Severnaya Armatura LLC', 'ООО «Северная Арматура»'), country: 'RU', domains: ['sevarm.example'],
      inn: '7814567895', kpp: '781401001', ogrn: '1137847098765', ids: {}, legalAddress: L('6 Sofiyskaya St, St Petersburg 192236, Russia', '192236, г. Санкт-Петербург, ул. Софийская, д. 6'),
      bankName: L('Nevsky Industrial Bank', 'АО «Невский Промышленный Банк»'), bik: '044030888', account: '40702810500000098765', corrAccount: '30101810100000000888',
      website: 'sevarm.example', phones: ['+7 (812) 555-77-00'] },
  }
  // People on the cards (person → company key).
  const contacts = ['femke', 'ruben', 'jonas', 'rachel', 'oliver', 'markus', 'piotr', 'daniel', 'li', 'dmitry']

  return { ru, L, D, D2, org, users, U, mailboxes, annaAccounts, gmail, signatures, startTemplates, P, C, contacts }
}
