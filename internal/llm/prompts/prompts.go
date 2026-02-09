package prompts

import "fmt"

// SystemClassifier is the system prompt for news classification.
const SystemClassifier = `Ты — финансовый аналитик, специализирующийся на российском фондовом рынке.
Твоя задача — классифицировать новости и извлечь ключевую информацию.
Отвечай ТОЛЬКО в формате JSON, без пояснений.`

// ClassifyNews generates a prompt for classifying a news item.
func ClassifyNews(title, content string) string {
	return fmt.Sprintf(`Проанализируй следующую новость и верни JSON:

Заголовок: %s
Текст: %s

Верни JSON со следующими полями:
{
  "category": "<одна из: CORP_EARNINGS, CORP_DIVIDEND, CORP_MA, CORP_MANAGEMENT, CORP_LEGAL, CORP_DEBT, CORP_RATING, CB_RATE, CB_POLICY, MACRO_INFLATION, MACRO_GDP, MACRO_EMPLOYMENT, GEO_SANCTIONS, GEO_DIPLOMACY, GEO_CONFLICT, GEO_TRADE, COMMODITY_OIL, COMMODITY_GAS, COMMODITY_METAL, COMMODITY_AGRO, REGULATION, WEATHER, TECH>",
  "sentiment": <число от -1.0 до 1.0, где -1 крайне негативно, 0 нейтрально, 1 крайне позитивно>,
  "urgency": <число от 1 до 5, где 5 критически срочно>,
  "reliability": <число от 0.0 до 1.0, насколько достоверен источник>,
  "summary": "<краткое резюме на русском, 1-2 предложения>",
  "key_facts": ["факт 1", "факт 2"],
  "tickers": ["SBER", "GAZP"],
  "sectors": ["BANKS", "OIL_GAS"],
  "regions": ["RU", "US"]
}`, title, content)
}

// SystemImpactAnalyzer is the system prompt for deep impact analysis (Claude).
const SystemImpactAnalyzer = `Ты — старший финансовый аналитик с глубоким знанием российского рынка.
Твоя задача — оценить влияние новости на конкретные финансовые инструменты.
Учитывай межсекторальные связи, цепочки поставок, макроэкономический контекст.
Отвечай ТОЛЬКО в формате JSON.`

// AnalyzeImpact generates a prompt for deep impact analysis.
func AnalyzeImpact(title, content, category string, tickers []string) string {
	return fmt.Sprintf(`Оцени влияние следующей новости на финансовые инструменты:

Заголовок: %s
Текст: %s
Категория: %s
Упомянутые тикеры: %v

Для каждого затронутого инструмента верни JSON массив:
[
  {
    "ticker": "SBER",
    "entity_type": "company",
    "impact_direction": <-1, 0, или 1>,
    "impact_magnitude": <0.0 до 1.0>,
    "impact_timeframe": "<IMMEDIATE|SHORT|MEDIUM|LONG>",
    "confidence": <0.0 до 1.0>,
    "reasoning": "<почему именно такое влияние, на русском>"
  }
]

Учти:
- Прямые и косвенные связи (поставщики, конкуренты, сектор)
- Макроэкономический контекст (ставка ЦБ, инфляция, курс рубля)
- Для облигаций: кредитный риск, изменение доходностей
- Для сырья: баланс спроса/предложения`, title, content, category, tickers)
}

// SystemDailyDigest is the system prompt for daily digest generation (Claude).
const SystemDailyDigest = `Ты — финансовый журналист и аналитик.
Твоя задача — составить ежедневный обзор российского рынка.
Пиши кратко, по существу, с конкретными цифрами и фактами.
Структурируй по секторам и уровню важности.`

// GenerateDigest generates a prompt for daily market digest.
func GenerateDigest(newsJSON string) string {
	return fmt.Sprintf(`На основе следующих новостей за день составь краткий обзор рынка:

%s

Формат обзора:
1. **Главное за день** (3-5 пунктов)
2. **По секторам** (ключевые события для каждого затронутого сектора)
3. **На что обратить внимание завтра** (2-3 пункта)
4. **Общий тон рынка**: <бычий/медвежий/нейтральный> с кратким обоснованием`, newsJSON)
}
