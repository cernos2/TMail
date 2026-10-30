-- Prompt fixes after the first real use (only where an administrator has not edited the prompt):
-- clarifications are what the SENDER must tell us; the reply never adds a sign-off.
UPDATE ai_prompts SET system = replace(system,
  'Separately list what is worth clarifying with the sender if the letter lacks information (delivery address, quantity, model, company details, etc.). Do not repeat what is already in the letter.',
  'Separately list what is worth clarifying with the sender: only information that THE SENDER must provide (delivery address, quantity, model, company details, their own deadlines, etc.) if it is missing from the letter. Do not include our internal checks (stock in our warehouse, prices, delivery times): those are questions for the employee above. Do not repeat what is already in the letter.')
WHERE key = 'reply_questions' AND updated_by = '';

UPDATE ai_prompts SET system = replace(system,
  '- Do not add a signature or contact details: the mail program adds the signature itself.',
  '- Do not add a signature, contact details or a closing line with a name ("Best regards, …"): the mail program adds the signature itself. End the letter with its last sentence of substance.')
WHERE key = 'reply' AND updated_by = '';
