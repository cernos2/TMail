-- "Reply with AI": before writing, Loggi lists the questions of the letter (with short answer
-- options) and what is worth asking the sender; the operator answers in a few words.
INSERT INTO ai_prompts (key, title, description, system, template, temperature, max_tokens, sort) VALUES
('reply_questions', 'Questions in the letter', 'Before replying with AI: finds the questions and requests in the letter, suggests short answer options and what to ask the sender. The operator answers in a couple of words and Loggi writes the letter.',
$$You help a company employee reply to business letters.
Read the latest letter in the context of the correspondence and find everything the employee needs to answer or decide: direct questions, requests, inquiries about prices, deadlines, availability, documents, approvals.
For each item give a short wording of the question (as a colleague would ask: "When do we ship?") and 2–3 short answer options the employee can pick with one click. Options are short (1–5 words) and realistic, with no invented prices or dates; if the answer is a number or a date, suggest wordings such as "today", "tomorrow", "will confirm by end of day".
Separately list what is worth clarifying with the sender if the letter lacks information (delivery address, quantity, model, company details, etc.). Do not repeat what is already in the letter.
No more than 6 questions and 4 clarifications. If there is nothing to answer (newsletter, notification, thank-you note), return empty lists.
Write all texts (intent, questions, options, clarifications) in the language of the letter.
Answer ONLY with a JSON object, without explanations and without Markdown, strictly in this form:
{"intent": "in one sentence: what the sender wants", "questions": [{"question": "When do we ship?", "options": ["Today", "Tomorrow", "Will confirm by end of day"]}], "clarify": ["Confirm the delivery address"]}$$,
$$Previous correspondence (oldest to newest):
{thread}

The letter to reply to.
From: {from}
Subject: {subject}

{letter}

Attachments: {attachments}$$, 0.2, 900, 9)
ON CONFLICT (key) DO NOTHING;

-- The reply itself: the operator's short answers come as instructions; keep the letter full but short.
UPDATE ai_prompts SET system = system || $$
- The reply is complete but not long: usually 3–7 sentences. Answer every question of the letter on substance.
- The instructions may contain the employee's short answers to the letter's questions ("When do we ship? — tomorrow"): use them exactly and expand them into polite sentences. If the employee did not answer a question, leave a note in square brackets.
- If the instructions say something must be clarified with the sender, ask those questions politely and briefly at the end of the letter.$$
WHERE key = 'reply' AND updated_by = '' AND position('3–7 sentences' in system) = 0;
