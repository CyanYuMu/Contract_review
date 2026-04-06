export const removeQuotes = (text: string): string => {
  if (!text) return text;
  let cleaned = text;
  const quoteChars = [
    "\u201C",
    "\u201D",
    "\u201E",
    "\u201F",
    "\u2018",
    "\u2019",
    "\u201A",
    "\u201B",
    "\u00AB",
    "\u00BB",
    "\u2039",
    "\u203A",
    "\u201E",
    "\u201A",
    "\u0022",
    "\u0027",
    "\u301D",
    "\u301E",
    "\uFF02",
    "\uFF07",
  ];
  quoteChars.forEach((char) => {
    cleaned = cleaned.replace(new RegExp(char, "g"), "");
  });
  cleaned = cleaned.replace(/[""''""]+/g, "").trim();
  return cleaned;
};

export const createSearchStrategies = (originalContent: string) => {
  // 先移除引号和换行符
  const cleaned = removeQuotes(originalContent.trim()).replace(/[\r\n]+/g, "");
  return [
    () => cleaned,
    () => cleaned.replace(/\s+/g, " "),
    () => cleaned.replace(/\s+/g, ""),
    () => (cleaned.length > 100 ? cleaned.substring(0, 100) : cleaned),
    () => (cleaned.length > 80 ? cleaned.substring(0, 80) : cleaned),
    () => (cleaned.length > 60 ? cleaned.substring(0, 60) : cleaned),
    () => (cleaned.length > 40 ? cleaned.substring(0, 40) : cleaned),
    () => (cleaned.length > 30 ? cleaned.substring(0, 30) : cleaned),
    () => (cleaned.length > 20 ? cleaned.substring(0, 20) : cleaned),
    () => cleaned,
    () => cleaned.replace(/\s+/g, " "),
  ];
};
