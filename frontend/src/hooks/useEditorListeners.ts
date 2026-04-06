import { useEffect, useCallback, useRef } from 'react';
import { debounce } from '@/lib/canvas-editor/utils';

export interface EditorState {
  fontFamily: string;
  fontSize: string;
  textColor: string;
  highlightColor: string;
  isBold: boolean;
  isItalic: boolean;
  isUnderline: boolean;
  titleLevel: string;
  rowMargin: string;
  listType: string;
  alignment: string;
  pageMode: string;
  pageScale: number;
  paperSize: string;
  paperDirection: string;
  wordCount: number;
  pageNo: number;
  pageSize: number;
  rowNo: number;
  colNo: number;
}

/* eslint-disable @typescript-eslint/no-explicit-any */
export interface UseEditorListenersProps {
  editor: any;
  onStateChange: (state: Partial<EditorState>) => void;
}

export const useEditorListeners = ({ editor, onStateChange }: UseEditorListenersProps) => {
  const stateRef = useRef<EditorState>({
    fontFamily: '微软雅黑',
    fontSize: '小四',
    textColor: '#000000',
    highlightColor: '#ffff00',
    isBold: false,
    isItalic: false,
    isUnderline: false,
    titleLevel: '正文',
    rowMargin: '1.5',
    listType: '',
    alignment: 'left',
    pageMode: 'paging',
    pageScale: 100,
    paperSize: 'A4',
    paperDirection: 'vertical',
    wordCount: 0,
    pageNo: 1,
    pageSize: 1,
    rowNo: 0,
    colNo: 0,
  });

  const updateState = useCallback((updates: Partial<EditorState>) => {
    stateRef.current = { ...stateRef.current, ...updates };
    onStateChange(updates);
  }, [onStateChange]);

  useEffect(() => {
    if (!editor) return;

    // 使用防抖触发选区样式同步，降低频繁触发成本
    const triggerRangeStyleSync = debounce(() => {
      try {
        if (editor.command && editor.command.setRangeStyle) {
          editor.command.setRangeStyle();
        }
      } catch (error) {
        console.warn('触发选区样式同步失败:', error);
      }
    }, 80);

    // 内容变化监听器（使用防抖优化性能）
    const handleContentChange = debounce(() => {
      editor.command.getWordCount().then((count: number) => {
        updateState({ wordCount: count || 0 });
      });
    }, 300);

    // 页面变化监听器
    const handlePageChange = (payload: number) => {
      updateState({ pageNo: payload + 1 });
    };

    // 页面大小变化监听器
    const handlePageSizeChange = (payload: number) => {
      updateState({ pageSize: payload });
    };

    // 页面缩放变化监听器
    const handlePageScaleChange = (payload: number) => {
      updateState({ pageScale: Math.floor(payload * 100) });
    };

    // 页面模式变化监听器
    const handlePageModeChange = (payload: string) => {
      updateState({ pageMode: payload });
    };

    // 可见页码列表变化监听器
    const handleVisiblePageNoListChange = (payload: number[]) => {
      console.log('可见页码:', payload);
    };

    // 选区样式变化监听器 - 核心监听器
    const handleRangeStyleChange = (payload: any) => {

      const updates: Partial<EditorState> = {};

      // 字体变化
      if (payload.font) {
        updates.fontFamily = payload.font;
      }

      // 字号变化
      if (payload.size) {
        const sizeMap: { [key: number]: string } = {
          56: '初号', 48: '小初', 34: '一号', 32: '小一',
          29: '二号', 24: '小二', 21: '三号', 20: '小三',
          18: '四号', 16: '小四', 14: '五号', 12: '小五'
        };
        // 未命中映射时，使用像素值降级显示，避免误导为固定“小四”
        updates.fontSize = sizeMap[payload.size] || `${payload.size}px`;
      }

      // 文本样式变化
      updates.isBold = !!payload.bold;
      updates.isItalic = !!payload.italic;
      updates.isUnderline = !!payload.underline;

      // 颜色变化
      if (payload.color) {
        updates.textColor = payload.color;
      }
      if (payload.highlight) {
        updates.highlightColor = payload.highlight;
      }

      // 对齐方式变化
      if (payload.rowFlex) {
        updates.alignment = payload.rowFlex;
      }

      // 标题级别变化
      if (payload.level) {
        const levelMap: { [key: string]: string } = {
          'first': '标题1',
          'second': '标题2', 
          'third': '标题3',
          'fourth': '标题4',
          'fifth': '标题5',
          'sixth': '标题6'
        };
        updates.titleLevel = levelMap[payload.level] || '正文';
      } else {
        updates.titleLevel = '正文';
      }

      // 行间距变化
      if (payload.rowMargin) {
        updates.rowMargin = payload.rowMargin.toString();
      }

      // 列表类型变化
      if (payload.listType) {
        updates.listType = payload.listType;
      } else {
        updates.listType = '';
      }

      // 行列信息更新
      if (payload.rowNo !== undefined) {
        updates.rowNo = payload.rowNo + 1;
      }
      if (payload.colNo !== undefined) {
        updates.colNo = payload.colNo + 1;
      }

      updateState(updates);
    };

    // 记录已有监听器，合并而非覆盖
    const prevListeners = {
      contentChange: editor.listener?.contentChange as (() => void) | null,
      intersectionPageNoChange: editor.listener?.intersectionPageNoChange as ((payload: number) => void) | null,
      pageSizeChange: editor.listener?.pageSizeChange as ((payload: number) => void) | null,
      pageScaleChange: editor.listener?.pageScaleChange as ((payload: number) => void) | null,
      pageModeChange: editor.listener?.pageModeChange as ((payload: string) => void) | null,
      visiblePageNoListChange: editor.listener?.visiblePageNoListChange as ((payload: number[]) => void) | null,
      rangeStyleChange: editor.listener?.rangeStyleChange as ((payload: any) => void) | null,
    };

    // 包装合并后的监听器引用，便于清理时比对
    const mergedListeners = {
      contentChange: () => {
        if (prevListeners.contentChange) {
          prevListeners.contentChange();
        }
        handleContentChange();
      },
      intersectionPageNoChange: (payload: number) => {
        if (prevListeners.intersectionPageNoChange) {
          prevListeners.intersectionPageNoChange(payload);
        }
        handlePageChange(payload);
      },
      pageSizeChange: (payload: number) => {
        if (prevListeners.pageSizeChange) {
          prevListeners.pageSizeChange(payload);
        }
        handlePageSizeChange(payload);
      },
      pageScaleChange: (payload: number) => {
        if (prevListeners.pageScaleChange) {
          prevListeners.pageScaleChange(payload);
        }
        handlePageScaleChange(payload);
      },
      pageModeChange: (payload: string) => {
        if (prevListeners.pageModeChange) {
          prevListeners.pageModeChange(payload);
        }
        handlePageModeChange(payload);
      },
      visiblePageNoListChange: (payload: number[]) => {
        if (prevListeners.visiblePageNoListChange) {
          prevListeners.visiblePageNoListChange(payload);
        }
        handleVisiblePageNoListChange(payload);
      },
      rangeStyleChange: (payload: any) => {
        if (prevListeners.rangeStyleChange) {
          prevListeners.rangeStyleChange(payload);
        }
        handleRangeStyleChange(payload);
      },
    };

    // 注册所有监听器（合并后）
    editor.listener.contentChange = mergedListeners.contentChange;
    editor.listener.intersectionPageNoChange = mergedListeners.intersectionPageNoChange;
    editor.listener.pageSizeChange = mergedListeners.pageSizeChange;
    editor.listener.pageScaleChange = mergedListeners.pageScaleChange;
    editor.listener.pageModeChange = mergedListeners.pageModeChange;
    editor.listener.visiblePageNoListChange = mergedListeners.visiblePageNoListChange;
    editor.listener.rangeStyleChange = mergedListeners.rangeStyleChange;

    // 初始化内容
    handleContentChange();

    // 触发一次初始选区样式同步
    triggerRangeStyleSync();

    // 编辑器交互事件处理
    const handleEditorInteraction = () => {
      triggerRangeStyleSync();
    };

    // 添加编辑器容器事件监听
    const editorContainer = editor.command.getContainer();
    if (editorContainer) {
      editorContainer.addEventListener('click', handleEditorInteraction);
      editorContainer.addEventListener('keyup', handleEditorInteraction);
      editorContainer.addEventListener('keydown', handleEditorInteraction);
      editorContainer.addEventListener('mousedown', handleEditorInteraction);
      editorContainer.addEventListener('mouseup', handleEditorInteraction);
      editorContainer.addEventListener('focus', handleEditorInteraction);
      editorContainer.addEventListener('blur', handleEditorInteraction);
    }

    // 添加全局事件监听
    const handleGlobalKeyUp = (e: KeyboardEvent) => {
      // 当用户使用键盘快捷键时也触发状态更新
      if (e.ctrlKey || e.metaKey || e.altKey) {
        handleEditorInteraction();
      }
    };

    const handleDocumentSelectionChange = () => {
      handleEditorInteraction();
    };

    document.addEventListener('keyup', handleGlobalKeyUp);
    document.addEventListener('selectionchange', handleDocumentSelectionChange);

    // 清理函数
    return () => {
      if (editor.listener) {
        // 仅当监听器仍为本 Hook 注册的合并版本时，恢复之前的监听器，避免覆盖后续组件的设置
        if (editor.listener.contentChange === mergedListeners.contentChange) {
          editor.listener.contentChange = prevListeners.contentChange;
        }
        if (editor.listener.intersectionPageNoChange === mergedListeners.intersectionPageNoChange) {
          editor.listener.intersectionPageNoChange = prevListeners.intersectionPageNoChange;
        }
        if (editor.listener.pageSizeChange === mergedListeners.pageSizeChange) {
          editor.listener.pageSizeChange = prevListeners.pageSizeChange;
        }
        if (editor.listener.pageScaleChange === mergedListeners.pageScaleChange) {
          editor.listener.pageScaleChange = prevListeners.pageScaleChange;
        }
        if (editor.listener.pageModeChange === mergedListeners.pageModeChange) {
          editor.listener.pageModeChange = prevListeners.pageModeChange;
        }
        if (editor.listener.visiblePageNoListChange === mergedListeners.visiblePageNoListChange) {
          editor.listener.visiblePageNoListChange = prevListeners.visiblePageNoListChange;
        }
        if (editor.listener.rangeStyleChange === mergedListeners.rangeStyleChange) {
          editor.listener.rangeStyleChange = prevListeners.rangeStyleChange;
        }
      }

      const editorContainer = editor.command.getContainer();
      if (editorContainer) {
        editorContainer.removeEventListener('click', handleEditorInteraction);
        editorContainer.removeEventListener('keyup', handleEditorInteraction);
        editorContainer.removeEventListener('keydown', handleEditorInteraction);
        editorContainer.removeEventListener('mousedown', handleEditorInteraction);
        editorContainer.removeEventListener('mouseup', handleEditorInteraction);
        editorContainer.removeEventListener('focus', handleEditorInteraction);
        editorContainer.removeEventListener('blur', handleEditorInteraction);
      }

      document.removeEventListener('keyup', handleGlobalKeyUp);
      document.removeEventListener('selectionchange', handleDocumentSelectionChange);
    };
  }, [editor, updateState]);

  return stateRef.current;
};
