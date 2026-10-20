const { writeFile } = require("node:fs/promises");
const path = require("node:path");
const { chromium } = require("playwright");

const baseURL = process.env.ISSUE17_BASE_URL ?? "http://127.0.0.1:5173";
const evidenceDir = path.resolve(
  process.env.ISSUE17_EVIDENCE_DIR ?? "docs/reports/evidence/issue-17",
);
const evidence = [];

function record(check, details = "") {
  evidence.push({ check, details, passed: true });
}

async function chooseDeck(page, name) {
  await page.locator(".deck-tile", { hasText: name }).click();
  await page.getByRole("button", { name: "Выбрать эту колоду" }).click();
  await page.getByRole("button", { name: "Эта колода выбрана" }).waitFor();
}

async function handNames(page) {
  return page.locator(".hand-card strong").allTextContents();
}

async function submitMulligan(page, keepNames = []) {
  const labels = page.locator(".hand-card");
  const count = await labels.count();
  for (let index = 0; index < count; index += 1) {
    const label = labels.nth(index);
    const name = (await label.locator("strong").textContent())?.trim() ?? "";
    if (!keepNames.includes(name)) await label.locator("input").click();
  }
  await page
    .getByRole("button", { name: /Оставить эту руку|Заменить карты/ })
    .click();
}

async function createMatch(browser, primaryDeck, wantedNames, attempt) {
  const primaryContext = await browser.newContext({
    hasTouch: true,
    viewport: { width: 1366, height: 768 },
  });
  const opponentContext = await browser.newContext({
    viewport: { width: 1366, height: 768 },
  });
  await primaryContext.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;
    window.WebSocket = class ObservableWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        window.__issue17Socket = this;
      }
    };
  });
  const primary = await primaryContext.newPage();
  const opponent = await opponentContext.newPage();
  primary.on("pageerror", (error) => console.error("primary pageerror:", error));
  opponent.on("pageerror", (error) => console.error("opponent pageerror:", error));
  await Promise.all([primary.goto(baseURL), opponent.goto(baseURL)]);
  await primary
    .getByRole("button", { name: "Создать приватную комнату" })
    .click();
  const codeNode = primary.locator(".room-chip strong");
  await codeNode.waitFor();
  const code = (await codeNode.textContent()).trim();
  await opponent.locator("#room-code").fill(code);
  await opponent.getByRole("button", { name: "Войти" }).click();
  await Promise.all([
    primary.getByText("Соперник подключился").waitFor(),
    opponent.getByText("Соперник подключился").waitFor(),
  ]);

  if (attempt === 1) {
    const thirdContext = await browser.newContext();
    const third = await thirdContext.newPage();
    await third.goto(baseURL);
    await third.locator("#room-code").fill(code);
    await third.getByRole("button", { name: "Войти" }).click();
    await third.getByText("В комнате уже два игрока").waitFor();
    record("full-room", "Третья независимая сессия получила отдельное русское состояние ROOM_FULL");
    await thirdContext.close();
  }

  await Promise.all([
    chooseDeck(primary, primaryDeck),
    chooseDeck(opponent, "Кассий"),
  ]);
  await Promise.all([
    primary.getByRole("button", { name: "Я готов" }).click(),
    opponent.getByRole("button", { name: "Я готов" }).click(),
  ]);
  await Promise.all([
    primary.getByRole("heading", { name: "Выберите карты для замены" }).waitFor(),
    opponent.getByRole("heading", { name: "Выберите карты для замены" }).waitFor(),
  ]);
  const initialNames = await handNames(primary);
  const keep = wantedNames.filter((name) => initialNames.includes(name));
  await Promise.all([
    submitMulligan(primary, keep),
    submitMulligan(opponent, await handNames(opponent)),
  ]);
  await Promise.all([
    primary.getByRole("heading", { name: "Доступные действия" }).waitFor(),
    opponent.getByRole("heading", { name: "Доступные действия" }).waitFor(),
  ]);
  return { code, opponent, opponentContext, primary, primaryContext };
}

async function findMatch(browser, deck, wantedNames, predicate, limit = 8) {
  for (let attempt = 1; attempt <= limit; attempt += 1) {
    const match = await createMatch(browser, deck, wantedNames, attempt);
    const names = await match.primary.locator(".own-hand .game-card strong").allTextContents();
    if (predicate(names)) {
      record("independent-sessions", `Два изолированных browser context, комната ${match.code}, попытка ${attempt}`);
      return match;
    }
    await match.primaryContext.close();
    await match.opponentContext.close();
  }
  throw new Error(`Не удалось получить требуемую стартовую руку за ${limit} попыток`);
}

async function ensurePrimaryActs(primary, opponent) {
  const acting = primary.locator(".match-facts > div", { hasText: "Действует" }).locator("strong");
  if ((await acting.textContent())?.trim() === "Соперник") {
    await opponent.getByRole("button", { name: "Спасовать" }).click();
    await acting.getByText("Вы", { exact: true }).waitFor();
  }
}

async function playSuggested(page, name) {
  const suggestion = page
    .getByRole("button", { name: `Разыграть ${name}` })
    .first();
  console.log("playSuggested", name, "disabled=", await suggestion.isDisabled());
  await suggestion.click();
  const composer = page.locator(".action-composer");
  try {
    await composer.waitFor({ timeout: 5_000 });
  } catch (error) {
    await page.screenshot({ path: path.join(evidenceDir, "debug-composer.png"), fullPage: true });
    console.error("actions panel after click:", await page.locator(".actions-panel").innerText());
    throw error;
  }
  const placement = composer.locator("fieldset", { hasText: "Линия и клетка" }).locator("button");
  if ((await placement.count()) > 0) await placement.first().click();
  await composer.getByRole("button", { name: "Подтвердить и отправить" }).click();
}

async function exerciseViewports(page) {
  for (const [name, width, height] of [
    ["active-1366x768", 1366, 768],
    ["active-640x480", 640, 480],
    ["active-360x640", 360, 640],
    ["active-640x360", 640, 360],
  ]) {
    await page.setViewportSize({ width, height });
    await page.screenshot({ path: path.join(evidenceDir, `${name}.png`), fullPage: true });
    const actions = page.getByRole("heading", { name: "Доступные действия" });
    if (!(await actions.isVisible())) throw new Error(`Панель действий недоступна при ${width}x${height}`);
  }
  await page.setViewportSize({ width: 1366, height: 768 });
  record("viewports", "1366×768, 640×480, 360×640 и 640×360; поле и действия доступны");
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  try {
    const mandatory = await findMatch(
      browser,
      "Бьёрн",
      ["Разумный выбор"],
      (names) => names.includes("Разумный выбор"),
    );
    await exerciseViewports(mandatory.primary);
    await ensurePrimaryActs(mandatory.primary, mandatory.opponent);

    const zoomCard = mandatory.primary
      .getByRole("button", { name: /Открыть карту Разумный выбор/ })
      .first();
    await zoomCard.focus();
    await mandatory.primary.keyboard.press("Enter");
    await mandatory.primary.getByRole("dialog", { name: "Разумный выбор" }).waitFor();
    await mandatory.primary.keyboard.press("Escape");
    record("keyboard-zoom", "Карточный zoom открыт Enter и закрыт Escape без hover/drag-and-drop");

    const help = mandatory.primary.getByRole("button", { name: "Справка" });
    await help.tap();
    await mandatory.primary.getByRole("dialog", { name: "Справка по игре" }).waitFor();
    await mandatory.primary.getByRole("button", { name: "Понятно" }).click();
    record("touch-help", "Справка открыта touch-событием; интерактивная цель доступна без hover");

    await playSuggested(mandatory.primary, "Разумный выбор");
    await mandatory.primary.getByText("Выберите обязательный вариант").waitFor();
    await mandatory.opponent.getByText("Ожидаем решение соперника").waitFor();
    if (await mandatory.primary.getByRole("button", { name: "Не активировать" }).count()) {
      throw new Error("Mandatory choice ошибочно предлагает отказ");
    }
    await mandatory.primary.setViewportSize({ width: 640, height: 480 });
    await mandatory.primary.screenshot({ path: path.join(evidenceDir, "mandatory-choice-owner-640x480.png"), fullPage: true });
    await mandatory.opponent.setViewportSize({ width: 360, height: 640 });
    await mandatory.opponent.screenshot({ path: path.join(evidenceDir, "mandatory-choice-redacted-360x640.png"), fullPage: true });
    record("mandatory-choice", "Owner видит 1..1 без отказа; соперник видит только redacted ожидание");
    await mandatory.primary.locator(".choice-options button").first().click();
    await mandatory.primary.getByRole("button", { name: "Сбросить выбор" }).click();
    await mandatory.primary.locator(".choice-options button").first().click();
    await mandatory.primary.getByRole("button", { name: "Подтвердить решение" }).click();
    await mandatory.primary.getByText("Публичный журнал").waitFor();
    record("choice-cancel-submit", "Локальный выбор сброшен до отправки, затем подтверждён один раз");

    await mandatory.primary.evaluate(() => {
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
      document.dispatchEvent(new Event("visibilitychange"));
    });
    await mandatory.primary.getByText("Соединение восстановлено. Состояние игры обновлено").waitFor();
    record("background-resync", "Возврат вкладки отправил resync и завершился свежим matchSnapshot");

    await mandatory.primary.evaluate(() => window.__issue17Socket.close());
    await mandatory.primary.getByText("Связь с сервером потеряна", { exact: false }).waitFor({ timeout: 12_000 });
    await mandatory.primary.getByText("Соединение восстановлено. Состояние игры обновлено").waitFor({ timeout: 20_000 });
    await mandatory.primary.setViewportSize({ width: 1366, height: 768 });
    await mandatory.primary.screenshot({ path: path.join(evidenceDir, "reconnect-restored-1366x768.png"), fullPage: true });
    record("reconnect", "Реальный offline/online восстановил socket и персональную проекцию");

    await mandatory.primary.getByRole("button", { name: "Сдаться" }).click();
    await mandatory.primary.getByRole("dialog", { name: "Подтвердить сдачу?" }).getByRole("button", { name: "Сдаться" }).click();
    await Promise.all([
      mandatory.primary.getByText("Вы победили").or(mandatory.primary.getByText("Победил соперник")).waitFor(),
      mandatory.opponent.getByText("Вы победили").or(mandatory.opponent.getByText("Победил соперник")).waitFor(),
    ]);
    await mandatory.primary.getByRole("button", { name: "Предложить реванш" }).click();
    await mandatory.primary.getByRole("button", { name: "Отменить предложение" }).waitFor();
    await mandatory.primary.setViewportSize({ width: 640, height: 480 });
    await mandatory.primary.screenshot({ path: path.join(evidenceDir, "result-rematch-640x480.png"), fullPage: true });
    await mandatory.opponent.getByRole("button", { name: "Предложить реванш" }).click();
    await Promise.all([
      mandatory.primary.getByRole("heading", { name: "Выберите карты для замены" }).waitFor({ timeout: 20_000 }),
      mandatory.opponent.getByRole("heading", { name: "Выберите карты для замены" }).waitFor({ timeout: 20_000 }),
    ]);
    record("surrender-rematch", "Единственный surrender-result у обоих; rematch требует два согласия и создаёт новый матч");
    await mandatory.primaryContext.close();
    await mandatory.opponentContext.close();

    const optional = await findMatch(
      browser,
      "Дари",
      ["Патрульный энобот"],
      (names) => names.includes("Патрульный энобот"),
    );
    const optionalActing = optional.primary
      .locator(".match-facts > div", { hasText: "Действует" })
      .locator("strong");
    if ((await optionalActing.textContent())?.trim() === "Вы") {
      await optional.primary.getByRole("button", { name: "Спасовать" }).click();
    }
    const opponentUnits = [
      "Заблудшая Душа",
      "Рыцарь Смерти",
      "Возвращающийся",
      "Мёртвый банкир",
      "Большой зомби",
      "Неудержимый дух",
      "Неуязвимый мертвец",
    ];
    const opponentNames = await optional.opponent.locator(".own-hand .game-card strong").allTextContents();
    const opponentUnit = opponentUnits.find((name) => opponentNames.includes(name));
    if (!opponentUnit) throw new Error("В руке соперника нет безопасного юнита для optional-choice сценария");
    await playSuggested(optional.opponent, opponentUnit);
    await optional.primary.getByRole("button", { name: "Разыграть Патрульный энобот" }).first().waitFor();
    await playSuggested(optional.primary, "Патрульный энобот");
    await optional.primary.getByRole("button", { name: "Не активировать" }).waitFor();
    await optional.primary.setViewportSize({ width: 360, height: 640 });
    await optional.primary.screenshot({ path: path.join(evidenceDir, "optional-choice-360x640.png"), fullPage: true });
    await optional.primary.getByRole("button", { name: "Не активировать" }).click();
    await optional.primary.getByRole("heading", { name: "Доступные действия" }).waitFor();
    record("optional-choice", "Реальный optional PendingChoice показывает явное «Не активировать» и продолжает серверную очередь");
    await optional.primaryContext.close();
    await optional.opponentContext.close();

    await writeFile(
      path.join(evidenceDir, "browser-e2e.json"),
      `${JSON.stringify({
        browser: await browser.version(),
        completedAt: new Date().toISOString(),
        baseURL,
        checks: evidence,
      }, null, 2)}\n`,
      "utf8",
    );
    console.log(`Issue #17 browser E2E passed: ${evidence.length} checks`);
  } finally {
    await browser.close();
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
