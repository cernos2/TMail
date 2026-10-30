// Canned answers of the AI assistant: good-looking results for each kind of request, chosen by
// what the request is about (no model is called).

export function aiAnswer(lang, key, input, me) {
  const ru = lang === 'ru'
  const L = (en, r) => (ru ? r : en)
  const subj = (input.subject || '').toLowerCase()
  const letter = input.letter || ''
  const from = (input.from || '').replace(/\s*<.*$/, '').trim()
  const first = from.split(/\s+/)[0] || L('colleague', 'коллега')
  const isTender = /40|seawater|забортн|tug|буксир|q-2611|2611/i.test(subj + ' ' + letter)
  const isVibration = /vibrat|вибрац/i.test(subj + letter)
  const isPo = /4471/.test(subj + letter)
  const isInvoice = /invoice|счёт|счет/i.test(subj)
  const isEnquiry = /in stock|в наличии|enquiry|вопрос с сайта/i.test(subj + letter)
  const myFirst = (me || '').split(/\s+/)[0]

  switch (key) {
    case 'summary':
      if (isTender) return L(
        `• Femke de Vries (Tidewater) likes offer Q-2611 and takes it to their technical committee on Monday.
• She asks: 4-week lead time for the first 10 pumps, a discount for all 40 at once, payment 30/70 instead of 50/50, a spare parts kit per 10 pumps.
• Wants a short call tomorrow afternoon.
• To do: agree the discount limit with Peter (4%, 6% with 50% upfront) and answer before the call.`,
        `• Елена Фролова (верфь) одобряет КП № 2611 и в понедельник выносит его на техсовет.
• Просит: срок 4 недели для первых 10 насосов, скидку за все 40 сразу, оплату 30/70 вместо 50/50, комплект ЗИП на каждые 10 насосов.
• Предлагает созвониться завтра после обеда.
• Что сделать: согласовать скидку с Петром (4%, 6% при 50% предоплаты) и ответить до звонка.`)
      if (isVibration) return L(
        `• Pump P3 (KP-200) at Tidewater vibrates up to 11 mm/s after a motor replacement.
• The coupling was not re-aligned; rotation is correct.
• Most likely cause: misalignment. Ruben asks for a site visit on Friday.`,
        `• Насос P3 (КП-200) на верфи после замены двигателя вибрирует до 11 мм/с.
• Центровку муфты не делали, направление вращения верное.
• Вероятная причина — расцентровка. Роман просит выезд в пятницу.`)
      if (isPo) return L(
        `• Maple Ridge received all 6 NW-EA250 actuators, no damage; the signed delivery note is attached.
• They ask for the invoice and the commissioning certificates.`,
        `• Водоканал получил все 6 приводов NW-EA250 без повреждений, подписанная накладная во вложении.
• Просят счёт-фактуру и паспорта на приводы.`)
      if (isInvoice) return L(`• Invoice INV-2026-0418 from Brightline Parts for EUR 12,016.00.\n• Due 25 November; matches order NW-PO-2611.`, `• Счёт № 418 от «Брайтлайн Партс» на 1 075 300,00 ₽.\n• Оплатить до 25 ноября, соответствует заказу НВ-2611.`)
      return L(`• ${from || 'The sender'} writes about "${input.subject || 'no subject'}".\n• ${firstSentence(letter) || 'No specific request.'}\n• No deadline or amount mentioned.`,
        `• ${from || 'Отправитель'} пишет на тему «${input.subject || 'без темы'}».\n• ${firstSentence(letter) || 'Конкретной просьбы нет.'}\n• Сроков и сумм не названо.`)

    case 'reply_questions':
      if (isTender) return JSON.stringify(L({
        intent: 'Femke is ready to take the offer to the technical committee and asks about lead time, discount, payment terms and spare parts.',
        questions: [
          { question: 'Can we deliver the first 10 pumps in 4 weeks?', options: ['Yes, 4 weeks', '5 weeks', 'No, 6–7 weeks'] },
          { question: 'What discount for all 40 pumps at once?', options: ['3%', '4%', '6% with 50% upfront'] },
          { question: 'Payment 30/70 instead of 50/50?', options: ['Accept', '40/60', 'Keep 50/50'] },
          { question: 'Spare parts kit for every 10 pumps?', options: ['Included free', 'At cost', 'Separate price'] },
          { question: 'Call tomorrow afternoon?', options: ['14:00', '15:00', 'Suggest Thursday'] },
        ],
        clarify: ['Which tugs get the first 10 pumps?', 'Do they need the class certificates before delivery?'],
      }, {
        intent: 'Елена готова вынести предложение на техсовет и спрашивает про срок, скидку, условия оплаты и ЗИП.',
        questions: [
          { question: 'Сможем поставить первые 10 насосов за 4 недели?', options: ['Да, 4 недели', '5 недель', 'Нет, 6–7 недель'] },
          { question: 'Какая скидка за все 40 насосов сразу?', options: ['3%', '4%', '6% при 50% предоплаты'] },
          { question: 'Оплата 30/70 вместо 50/50?', options: ['Согласны', '40/60', 'Оставить 50/50'] },
          { question: 'Комплект ЗИП на каждые 10 насосов?', options: ['Бесплатно', 'По себестоимости', 'Отдельной строкой'] },
          { question: 'Созвон завтра после обеда?', options: ['14:00', '15:00', 'Предложить четверг'] },
        ],
        clarify: ['На какие буксиры пойдут первые 10 насосов?', 'Нужны ли свидетельства РМРС до поставки?'],
      }))
      if (isVibration) return JSON.stringify(L({
        intent: 'Ruben sent the vibration readings and asks for a site visit on Friday.',
        questions: [{ question: 'Can Sofia come on Friday?', options: ['Yes, 9:00', 'Yes, afternoon', 'Next Monday'] }, { question: 'Is it a warranty case?', options: ['Yes, free of charge', 'Need to check'] }],
        clarify: ['Who will meet the engineer on site?'],
      }, {
        intent: 'Роман прислал показания вибрации и просит выезд в пятницу.',
        questions: [{ question: 'София сможет приехать в пятницу?', options: ['Да, к 9:00', 'Да, после обеда', 'В понедельник'] }, { question: 'Это гарантийный случай?', options: ['Да, бесплатно', 'Нужно проверить'] }],
        clarify: ['Кто встретит инженера на объекте?'],
      }))
      return JSON.stringify(L({ intent: `${first} writes about "${input.subject || ''}".`, questions: [{ question: 'Do we agree?', options: ['Yes', 'No', 'Need more details'] }], clarify: ['Ask for the deadline'] },
        { intent: `${first} пишет на тему «${input.subject || ''}».`, questions: [{ question: 'Мы согласны?', options: ['Да', 'Нет', 'Нужны подробности'] }], clarify: ['Уточнить срок'] }))

    case 'reply': {
      const instr = input.instructions || ''
      if (isTender) return L(
        `Dear Femke,

Thank you for your quick feedback — glad the offer works for you.

1. Lead time: we can deliver the first 10 pumps in 4 weeks from the down payment; the remaining 30 follow 3 weeks later.
2. Discount: for an order of all 40 pumps at once we offer 4%, or 6% if you choose 50% payment upfront.
3. Payment: we would prefer to keep 50/50; with 30/70 the 4% discount applies.
4. Spare parts: we will add a kit of seals and bearings for every 10 pumps at cost — EUR 1,180 per kit.

Tomorrow at 14:00 works well for a call. I will send an updated offer right after it.

Best regards,`,
        `Елена, добрый день!

Спасибо за быстрый ответ, рада, что предложение подходит.

1. Срок: первые 10 насосов поставим за 4 недели с даты предоплаты, остальные 30 — через 3 недели после них.
2. Скидка: при заказе всех 40 насосов сразу — 4%, а при 50% предоплаты — 6%.
3. Оплата: предпочли бы оставить 50/50; при оплате 30/70 действует скидка 4%.
4. ЗИП: добавим комплект уплотнений и подшипников на каждые 10 насосов по себестоимости — 104 000 ₽ за комплект.

Завтра в 14:00 удобно созвониться. Сразу после звонка пришлю обновлённое предложение.

С уважением,`)
      if (isVibration) return L(
        `Hello Ruben,

Thank you for the readings. 11 mm/s at the drive end after a motor replacement without re-alignment points clearly to misalignment of the coupling.

Sofia will be on site on Friday at 9:00 with a laser alignment kit; the visit is covered by the warranty. Until then, please keep P3 switched off and run on the standby pump.

Best regards,`,
        `Роман, добрый день!

Спасибо за показания. 11 мм/с со стороны привода после замены двигателя без центровки — явный признак расцентровки муфты.

София будет у вас в пятницу в 9:00 с лазерным центровщиком; выезд по гарантии, бесплатно. До этого, пожалуйста, не включайте P3 и работайте на резервном насосе.

С уважением,`)
      if (isEnquiry) return L(
        `Dear John,

Thank you for your enquiry. Yes, we have 2 × KP-300/45 in stock in Bristol. The price is EUR 6,850 per pump; we can ship within 2 working days, delivery to Avonmouth the next day.

Shall I send you a formal quote?

Best regards,`,
        `Иван, добрый день!

Спасибо за обращение. Да, 2 насоса КП-300/45 есть на складе в Санкт-Петербурге. Цена — 612 000 ₽ за насос с НДС, отгрузка в течение 2 рабочих дней, доставка на следующий день.

Подготовить официальное коммерческое предложение?

С уважением,`)
      return L(`Dear ${first},

Thank you for your message.${instr ? ' ' + instr.replace(/\s+/g, ' ').trim().replace(/^./, c => c.toUpperCase()).replace(/([^.!?])$/, '$1.') : ' We will look into it and get back to you shortly.'}

Best regards,`, `${first}, добрый день!

Спасибо за письмо.${instr ? ' ' + instr.replace(/\s+/g, ' ').trim().replace(/^./, c => c.toUpperCase()).replace(/([^.!?])$/, '$1.') : ' Разберёмся и вернёмся с ответом в ближайшее время.'}

С уважением,`)
    }
    case 'write': {
      const to = (input.to || '').replace(/\s*<.*$/, '').split(',')[0].trim().split(/\s+/)[0]
      return L(`Dear ${to || 'colleague'},

${input.instructions ? 'I am writing regarding ' + input.instructions.replace(/^\s*(write|tell|ask)\s+(him|her|them)?\s*(that|to)?\s*/i, '').replace(/[.\s]+$/, '') + '.' : 'I hope you are well.'}

Could you please confirm at your earliest convenience? I am happy to discuss the details by phone.

Best regards,`, `${to || 'Коллега'}, добрый день!

${input.instructions ? 'Пишу по поводу: ' + input.instructions.replace(/[.\s]+$/, '') + '.' : 'Надеюсь, у вас всё хорошо.'}

Подтвердите, пожалуйста, когда будет возможность. Готова обсудить детали по телефону.

С уважением,`)
    }
    case 'improve':
      return polish(input.draft || '', ru)
    case 'fix':
      return fixTypos(input.draft || '')
    case 'rephrase':
      return L(`In other words: ${(input.draft || '').trim()}`, `Иначе говоря: ${(input.draft || '').trim()}`).replace(/^(In other words|Иначе говоря): (.)/, (m, a, c) => c.toUpperCase()).trim()
    case 'dictate':
      return tidy(input.draft || '')
    case 'pick_template':
      return isEnquiry ? '1' : isPo ? '2' : isVibration ? '3' : '0'
    case 'autoreply':
      return ''
  }
  return L('Done.', 'Готово.')
}

const firstSentence = s => (s.replace(/\s+/g, ' ').match(/[^.!?]{12,200}[.!?]/) || [''])[0].trim()

const TYPOS = { teh: 'the', recieve: 'receive', recieved: 'received', adress: 'address', seperate: 'separate', untill: 'until', tommorow: 'tomorrow', wich: 'which', definately: 'definitely', occured: 'occurred',
  'пожалуйсто': 'пожалуйста', 'извените': 'извините', 'зделать': 'сделать', 'вобщем': 'в общем', 'сдесь': 'здесь', 'щас': 'сейчас', 'обезательно': 'обязательно', 'прийдёт': 'придёт', 'прийдет': 'придёт' }
function fixTypos(s) {
  let out = s.replace(/[\p{L}]+/gu, w => { const r = TYPOS[w.toLowerCase()]; return r ? (w[0] === w[0].toUpperCase() ? r[0].toUpperCase() + r.slice(1) : r) : w })
  out = out.replace(/\s+([,.!?])/g, '$1').replace(/([,.!?])(?=[\p{L}])/gu, '$1 ').replace(/ {2,}/g, ' ')
  return out.replace(/(^|[.!?]\s+)(\p{Ll})/gu, (m, a, c) => a + c.toUpperCase()).trim()
}
function polish(s, ru) {
  const t = fixTypos(s).replace(/\n{3,}/g, '\n\n')
  if (!t) return t
  const body = t.replace(/^(hi|hello|привет|здравствуйте)[^\n]*\n+/i, '')
  return (ru ? 'Добрый день!\n\n' : 'Hello,\n\n') + body.replace(/([^.!?])$/, '$1.') + (ru ? '\n\nБуду благодарна за ответ.' : '\n\nThank you in advance for your reply.')
}
function tidy(s) {
  const t = fixTypos(s.replace(/\b(um+|uh+|э+|ну+|короче)\b[,]?\s*/giu, ''))
  return t.replace(/([^.!?])$/, '$1.')
}

// Text of an auto-reply from the form (reason, dates, substitute).
export function autoReplyText(lang, f) {
  const ru = lang === 'ru'
  const fmt = d => d ? new Intl.DateTimeFormat(ru ? 'ru-RU' : 'en-GB', { day: 'numeric', month: 'long' }).format(new Date(d + 'T12:00:00')) : ''
  const why = { vacation: ru ? 'в отпуске' : 'on holiday', trip: ru ? 'в командировке' : 'on a business trip', sick: ru ? 'на больничном' : 'on sick leave',
    holiday: ru ? 'не работаю в праздничные дни' : 'away for the public holidays', dayoff: ru ? 'взяла выходной' : 'taking a day off', other: ru ? 'временно не в офисе' : 'out of the office' }[f.reason] || (ru ? 'не в офисе' : 'out of the office')
  const until = fmt(f.dateTo)
  const body = ru
    ? `Добрый день!\n\nСпасибо за письмо. Я ${why}${until ? ` до ${until}` : ''} и отвечу после возвращения.${f.substitute ? `\n\nПо срочным вопросам обращайтесь: ${f.substitute}.` : ''}\n\nСпасибо за понимание!`
    : `Hello,\n\nThank you for your message. I am ${why}${until ? ` until ${until}` : ''} and will reply when I am back.${f.substitute ? `\n\nFor urgent matters please contact ${f.substitute}.` : ''}\n\nThank you for your understanding.`
  const subject = ru ? (until ? `Не в офисе до ${until}` : 'Не в офисе') : (until ? `Out of office until ${until}` : 'Out of office')
  return { subject, body }
}
