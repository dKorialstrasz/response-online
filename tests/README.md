# Тесты

Unit-тесты находятся рядом с Go/TypeScript кодом. `tests/content` проверяет
CardDefinition, field-level evidence, ability/scenario links, различение
unknown/null/zero, права на изображения и транзитивное замыкание. Каталог
`smoke` содержит чёрный ящик для локального диагностического API. Исполняемые
gameplay conformance, integration и E2E наборы добавляются задачами, которые
реализуют соответствующую функциональность; scaffold не подменяет их пустыми
тестами.

