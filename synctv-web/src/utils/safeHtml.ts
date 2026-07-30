export const textToSafeHtml = (text: string): string => {
  const element = document.createElement("span");
  element.textContent = text;
  return element.outerHTML;
};

const escapedTextEntities: Record<string, string> = {
  "&amp;": "&",
  "&lt;": "<",
  "&gt;": ">",
  "&#34;": '"',
  "&#39;": "'"
};

export const decodeEscapedText = (text: string): string =>
  text.replace(/&(amp|lt|gt|#34|#39);/g, (entity) => escapedTextEntities[entity] ?? entity);
