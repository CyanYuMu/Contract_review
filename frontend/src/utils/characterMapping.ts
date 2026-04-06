/**
 * 字符级映射工具
 * 用于在渲染后的 DOM 中建立"纯文本字符索引 → DOM 位置"的映射
 */

export interface CharacterPosition {
  node: Node; // 文本节点
  offset: number; // 在该节点中的偏移量
  charIndex: number; // 在全文中的字符索引
}

export interface TextRange {
  startNode: Node;
  startOffset: number;
  endNode: Node;
  endOffset: number;
}

export class CharacterMapper {
  private container: HTMLElement;
  private characterMap: CharacterPosition[] = [];
  private fullText: string = "";
  private normalizedText: string = "";
  private searchCache = new Map<
    string,
    { start: number; end: number } | null
  >(); // 搜索结果缓存
  private readonly maxCacheSize = 100;

  constructor(container: HTMLElement) {
    this.container = container;
    this.buildMapping();
  }

  /**
   * 构建字符映射表
   */
  private buildMapping(): void {
    this.characterMap = [];
    let charIndex = 0;
    const textParts: string[] = [];

    const walker = document.createTreeWalker(
      this.container,
      NodeFilter.SHOW_TEXT,
      null
    );

    let node: Node | null;
    while ((node = walker.nextNode())) {
      const text = node.textContent || "";
      textParts.push(text);

      // 为每个字符建立映射
      for (let i = 0; i < text.length; i++) {
        this.characterMap.push({
          node,
          offset: i,
          charIndex: charIndex++,
        });
      }
    }

    this.fullText = textParts.join("");
    // 规范化文本：移除多余空白，但保留基本结构
    this.normalizedText = this.fullText.replace(/\s+/g, " ").trim();
  }

  /**
   * 获取完整文本
   */
  getFullText(): string {
    return this.fullText;
  }

  /**
   * 获取规范化后的文本
   */
  getNormalizedText(): string {
    return this.normalizedText;
  }

  /**
   * 在文本中查找字符串，返回字符索引范围
   */
  findText(
    searchText: string,
    options: {
      caseSensitive?: boolean;
      normalize?: boolean;
    } = {}
  ): { start: number; end: number }[] {
    const { caseSensitive = false, normalize = true } = options;
    const results: { start: number; end: number }[] = [];

    const sourceText = normalize ? this.normalizedText : this.fullText;
    const search = caseSensitive ? searchText : searchText.toLowerCase();
    const source = caseSensitive ? sourceText : sourceText.toLowerCase();

    let index = 0;
    while ((index = source.indexOf(search, index)) !== -1) {
      results.push({
        start: index,
        end: index + search.length,
      });
      index += search.length;
    }

    return results;
  }

  /**
   * 智能查找：尝试多种规范化优先级（带缓存）
   */
  smartFind(searchText: string): { start: number; end: number } | null {
    // 检查缓存
    if (this.searchCache.has(searchText)) {
      const cached = this.searchCache.get(searchText);
      if (cached !== undefined) {
        return cached;
      }
    }

    // 执行查找
    const result = this.smartFindInternal(searchText);

    // 缓存结果（包括 null）
    // 如果缓存已满，删除最早的条目
    if (this.searchCache.size >= this.maxCacheSize) {
      const firstKey = this.searchCache.keys().next().value;
      if (firstKey !== undefined) {
        this.searchCache.delete(firstKey);
      }
    }
    this.searchCache.set(searchText, result);

    return result;
  }

  /**
   * 内部查找方法（不使用缓存）
   */
  private smartFindInternal(
    searchText: string
  ): { start: number; end: number } | null {
    const strategies = [
      // 优先级1: 完全精确匹配
      () => this.findInText(searchText, this.fullText),

      // 优先级2: 规范化空格后匹配
      () => {
        const normalized = searchText.replace(/\s+/g, " ").trim();
        return this.findInText(normalized, this.normalizedText);
      },

      // 优先级3: 移除所有空白字符
      () => {
        const noSpace = searchText.replace(/\s+/g, "");
        const sourceNoSpace = this.fullText.replace(/\s+/g, "");
        const index = sourceNoSpace.indexOf(noSpace);
        if (index === -1) return null;

        // 反向映射到原始位置
        return this.mapNoSpaceIndexToOriginal(index, noSpace.length);
      },

      // 优先级4: 移除标点周围的空格
      () => {
        const cleaned = searchText
          .replace(/\s*([，。！？；：、])\s*/g, "$1")
          .replace(/\s+/g, " ")
          .trim();
        return this.findInText(cleaned, this.normalizedText);
      },

      // 优先级5: 全角转半角
      () => {
        const halfWidth = this.toHalfWidth(searchText);
        const sourceHalfWidth = this.toHalfWidth(this.fullText);
        const index = sourceHalfWidth
          .toLowerCase()
          .indexOf(halfWidth.toLowerCase());
        if (index === -1) return null;
        return { start: index, end: index + halfWidth.length };
      },

      // 优先级6: 分段匹配（前半部分）
      () => {
        if (searchText.length < 20) return null; // 太短不适合分段
        const half = Math.floor(searchText.length / 2);
        const firstHalf = searchText.substring(0, half);
        return this.findInText(firstHalf, this.fullText);
      },

      // 优先级7: 分段匹配（后半部分）
      () => {
        if (searchText.length < 20) return null;
        const half = Math.floor(searchText.length / 2);
        const secondHalf = searchText.substring(half);
        const result = this.findInText(secondHalf, this.fullText);
        if (!result) return null;
        // 尝试向前扩展到完整匹配
        const expandedStart = Math.max(0, result.start - half);
        return { start: expandedStart, end: result.end };
      },
    ];

    for (let i = 0; i < strategies.length; i++) {
      const result = strategies[i]();
      if (result) {
        return result;
      }
    }

    return null;
  }

  /**
   * 全角转半角
   */
  private toHalfWidth(str: string): string {
    return str
      .replace(/[Ａ-Ｚａ-ｚ０-９]/g, (char) => {
        return String.fromCharCode(char.charCodeAt(0) - 0xfee0);
      })
      .replace(/　/g, " "); // 全角空格转半角
  }

  /**
   * 在指定文本中查找
   */
  private findInText(
    search: string,
    source: string
  ): { start: number; end: number } | null {
    const index = source.toLowerCase().indexOf(search.toLowerCase());
    if (index === -1) return null;

    return {
      start: index,
      end: index + search.length,
    };
  }

  /**
   * 将无空格索引映射回原始索引
   */
  private mapNoSpaceIndexToOriginal(
    noSpaceIndex: number,
    length: number
  ): { start: number; end: number } | null {
    let currentNoSpaceIndex = 0;
    let startIndex = -1;
    let endIndex = -1;

    for (let i = 0; i < this.fullText.length; i++) {
      const char = this.fullText[i];

      if (!/\s/.test(char)) {
        if (currentNoSpaceIndex === noSpaceIndex && startIndex === -1) {
          startIndex = i;
        }

        currentNoSpaceIndex++;

        if (currentNoSpaceIndex === noSpaceIndex + length) {
          endIndex = i + 1;
          break;
        }
      }
    }

    if (startIndex === -1 || endIndex === -1) return null;

    return { start: startIndex, end: endIndex };
  }

  /**
   * 根据字符索引获取 DOM 范围
   */
  getRange(startIndex: number, endIndex: number): TextRange | null {
    if (startIndex < 0 || endIndex > this.characterMap.length) {
      return null;
    }

    const startPos = this.characterMap[startIndex];
    const endPos =
      this.characterMap[Math.min(endIndex - 1, this.characterMap.length - 1)];

    if (!startPos || !endPos) {
      return null;
    }

    return {
      startNode: startPos.node,
      startOffset: startPos.offset,
      endNode: endPos.node,
      endOffset: endPos.offset + 1, // +1 因为 Range.setEnd 是不包含的
    };
  }

  /**
   * 高亮指定范围
   */
  highlightRange(startIndex: number, endIndex: number): boolean {
    const textRange = this.getRange(startIndex, endIndex);
    if (!textRange) return false;

    try {
      const range = document.createRange();
      range.setStart(textRange.startNode, textRange.startOffset);
      range.setEnd(textRange.endNode, textRange.endOffset);

      const selection = window.getSelection();
      if (selection) {
        selection.removeAllRanges();
        selection.addRange(range);
      }

      // 滚动到视图
      const startElement = textRange.startNode.parentElement;
      if (startElement) {
        startElement.scrollIntoView({
          behavior: "smooth",
          block: "center",
        });
      }

      return true;
    } catch (error) {
      console.error("高亮失败:", error);
      return false;
    }
  }

  /**
   * 重建映射（当 DOM 更新时调用）
   */
  rebuild(): void {
    this.buildMapping();
    this.searchCache.clear();
  }

  /**
   * 清空搜索缓存
   */
  clearCache(): void {
    this.searchCache.clear();
  }

  /**
   * 获取缓存统计信息
   */
  getCacheStats(): { size: number; keys: string[] } {
    return {
      size: this.searchCache.size,
      keys: Array.from(this.searchCache.keys()),
    };
  }
}
