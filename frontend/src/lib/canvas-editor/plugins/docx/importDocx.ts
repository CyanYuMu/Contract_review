import {Command} from '../../editor'

declare module '../../editor' {
    interface Command {
        executeImportDocx(options: IImportDocxOption): void
    }
}

export interface IImportDocxOption {
    arrayBuffer: ArrayBuffer
}

export default function (command: Command) {
    return async function (options: IImportDocxOption) {
        if (typeof window === 'undefined' || typeof document === 'undefined') {
            throw new Error('docx 导入功能仅在客户端可用')
        }

        const {arrayBuffer} = options

        const editorContainer = command.getContainer()
        if (!editorContainer) {
            throw new Error('无法找到编辑器容器')
        }

        const parentElement = editorContainer.parentElement
        const parentRect = parentElement?.getBoundingClientRect() || {width: 0, height: 0}
        const containerRect = editorContainer.getBoundingClientRect()

        let scrollParent: HTMLElement | null = null
        let originalOverflow = ''
        let currentParent = parentElement
        while (currentParent) {
            const style = window.getComputedStyle(currentParent)
            if (style.overflow === 'auto' || style.overflowY === 'auto') {
                scrollParent = currentParent as HTMLElement
                originalOverflow = style.overflow
                scrollParent.setAttribute('data-original-overflow', originalOverflow)
                scrollParent.style.overflow = 'hidden'
                break
            }
            currentParent = currentParent.parentElement
        }

        editorContainer.style.cssText = `
      position: relative;
      width: 100%;
      height: 100%;
      min-height: ${Math.max(containerRect.height || parentRect.height || window.innerHeight - 200, 500)}px;
      overflow: hidden;
    `

        const canvasElements = editorContainer.querySelectorAll('*')
        canvasElements.forEach((el: Element) => {
            const htmlEl = el as HTMLElement
            if (htmlEl !== editorContainer && !htmlEl.id?.includes('docx-')) {
                htmlEl.style.display = 'none'
            }
        })

        const editableContainer = document.createElement('div')
        editableContainer.id = 'docx-editable-container'
        editableContainer.contentEditable = 'true'

        editableContainer.style.cssText = `
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      bottom: 0;
      width: 100%;
      height: 100%;
      background: #fff;
      padding: 20px;
      outline: none;
      overflow-y: auto;
      overflow-x: hidden;
      z-index: 1000;
      box-sizing: border-box;
    `

        const styleContainer = document.createElement('div')
        styleContainer.style.cssText = `
      position: absolute;
      top: 0;
      left: 0;
      pointer-events: none;
      z-index: 999;
    `

        editorContainer.appendChild(editableContainer)
        editorContainer.appendChild(styleContainer)

        try {
            if (typeof window === 'undefined') {
                throw new Error('docx-preview 仅在客户端可用')
            }

            const docx = await import('docx-preview')
            await docx.renderAsync(
                arrayBuffer,
                editableContainer,
                styleContainer,
                {
                    className: 'docx',
                    inWrapper: true,
                    ignoreWidth: false,
                    ignoreHeight: true,
                    ignoreFonts: false,
                    breakPages: true,
                    ignoreLastRenderedPageBreak: true,
                    experimental: false,
                    trimXmlDeclaration: true,
                    useBase64URL: true,
                    renderChanges: false,
                    renderHeaders: true,
                    renderFooters: true,
                    renderFootnotes: true,
                    renderEndnotes: true,
                    renderComments: false,
                    renderAltChunks: true,
                    debug: false
                }
            )

            // 修改页码
            editableContainer.querySelectorAll('.docx-wrapper > .docx').forEach((el, idx) => {
                const index = el.querySelector('footer > p > span:nth-child(2)')
                if (index) index.innerHTML = `${idx + 1}`
            })

            editableContainer.contentEditable = 'true'
            const editStyle = document.createElement('style')
            editStyle.id = 'docx-editable-styles'
            editStyle.textContent = `
        #docx-editable-container {
          cursor: text;
        }
        #docx-editable-container:focus {
          outline: none;
        }
        #docx-editable-container * {
          user-select: text;
        }
        #docx-editable-container .docx-wrapper {
          width: 100% !important;
          max-width: 100% !important;
          padding: 0 !important;
          margin: 0 !important;
          overflow: visible !important;
        }
        #docx-editable-container p {
          margin: 0.5em 0 !important;
          line-height: 1.5 !important;
        }
        #docx-editable-container table {
          width: 100% !important;
          max-width: 100% !important;
          border-collapse: collapse !important;
          margin: 1em 0 !important;
        }
        #docx-editable-container {
          overflow-y: auto !important;
          overflow-x: hidden !important;
        }
        #docx-editable-container .docx-wrapper,
        #docx-editable-container [class*="docx"] {
          overflow: visible !important;
        }
        #docx-editable-container * {
          max-width: 100% !important;
          box-sizing: border-box !important;
        }
         #docx-editable-container::selection,
        #docx-editable-container *::selection {
          background: #c7c7c7 !important;
          color: inherit;
        }
        
      `
            const oldStyle = document.getElementById('docx-editable-styles')
            if (oldStyle) {
                oldStyle.remove()
            }
            document.head.appendChild(editStyle)

            await new Promise(resolve => setTimeout(resolve, 100))

            const docxWrapper = editableContainer.querySelector('.docx-wrapper')
            if (docxWrapper) {
                ;(docxWrapper as HTMLElement).style.overflow = 'visible'
                ;(docxWrapper as HTMLElement).style.height = 'auto'
                ;(docxWrapper as HTMLElement).style.maxHeight = 'none'
            }
            const allElements = editableContainer.querySelectorAll('*')
            allElements.forEach((el: Element) => {
                const htmlEl = el as HTMLElement
                if (htmlEl !== editableContainer) {
                    const computedOverflow = window.getComputedStyle(htmlEl).overflow
                    if (computedOverflow === 'auto' || computedOverflow === 'scroll') {
                        htmlEl.style.overflow = 'visible'
                    }
                }
            })

            const originalCommand = command as any

            let editorListener: any = null
            let editorEventBus: any = null

            try {
                const adapt = (originalCommand as any).adapt
                if (adapt?.draw) {
                    editorListener = adapt.draw.listener
                    editorEventBus = adapt.draw.eventBus
                }
            } catch (e) {
                // 忽略访问错误
            }

            const getEditorInstance = (): any => {
                try {
                    const adapt = (originalCommand as any).adapt
                    if (adapt?.draw) {
                        const draw = adapt.draw
                        if (draw.listener && draw.eventBus) {
                            return {
                                listener: draw.listener,
                                eventBus: draw.eventBus
                            }
                        }
                    }
                } catch (e) {
                    // 忽略访问错误
                }
                return null
            }
            const originalMethods: any = {
                executeBold: originalCommand.executeBold,
                executeItalic: originalCommand.executeItalic,
                executeUnderline: originalCommand.executeUnderline,
                executeStrikeout: originalCommand.executeStrikeout,
                executeColor: originalCommand.executeColor,
                executeHighlight: originalCommand.executeHighlight,
                executeFont: originalCommand.executeFont,
                executeSize: originalCommand.executeSize,
                executeSizeAdd: originalCommand.executeSizeAdd,
                executeSizeMinus: originalCommand.executeSizeMinus,
                setRangeStyle: originalCommand.setRangeStyle,
            }

            const getSelection = () => {
                const selection = window.getSelection()
                if (!selection || selection.rangeCount === 0) return null
                return selection.getRangeAt(0)
            }

            const isSelectionInEditable = () => {
                const selection = window.getSelection()
                if (!selection || selection.rangeCount === 0) return false
                const range = selection.getRangeAt(0)
                return editableContainer.contains(range.commonAncestorContainer)
            }

            let savedRange: Range | null = null

            const saveSelection = () => {
                const selection = window.getSelection()
                if (selection && selection.rangeCount > 0) {
                    const range = selection.getRangeAt(0)
                    if (editableContainer.contains(range.commonAncestorContainer)) {
                        savedRange = range.cloneRange()
                        return true
                    }
                }
                savedRange = null
                return false
            }

            const restoreSelection = () => {
                if (!savedRange) return false

                const selection = window.getSelection()
                if (selection) {
                    try {
                        selection.removeAllRanges()
                        selection.addRange(savedRange.cloneRange())
                        editableContainer.focus()
                        return true
                    } catch (e) {
                        // 如果恢复失败，尝试创建一个新的选区
                        try {
                            const range = document.createRange()
                            range.setStart(savedRange.startContainer, savedRange.startOffset)
                            range.setEnd(savedRange.endContainer, savedRange.endOffset)
                            selection.removeAllRanges()
                            selection.addRange(range)
                            editableContainer.focus()
                            return true
                        } catch (e2) {
                            return false
                        }
                    }
                }
                return false
            }

            const execCommand = (command: string, value?: string) => {
                // 记录滚动位置与滚动行为，避免执行命令后跳到顶部/平滑动画造成闪烁
                const prevScrollTop = editableContainer.scrollTop
                const prevScrollBehavior = editableContainer.style.scrollBehavior
                editableContainer.style.scrollBehavior = 'auto'

                // 仅在需要时聚焦，避免额外的滚动与重绘
                if (document.activeElement !== editableContainer) {
                    editableContainer.focus()
                }

                const selection = window.getSelection()
                if (!selection) {
                    return
                }

                let rangeToUse: Range | null = null

                if (savedRange) {
                    try {
                        rangeToUse = savedRange.cloneRange()
                    } catch (e) {
                        rangeToUse = null
                    }
                }

                if (!rangeToUse) {
                    if (selection.rangeCount > 0) {
                        const currentRange = selection.getRangeAt(0)
                        if (editableContainer.contains(currentRange.commonAncestorContainer)) {
                            rangeToUse = currentRange.cloneRange()
                        }
                    }
                }

                if (!rangeToUse) {
                    rangeToUse = document.createRange()
                    rangeToUse.selectNodeContents(editableContainer)
                    rangeToUse.collapse(false)
                }

                // 避免不必要的选区抖动：只有在不同选区时才替换
                const needReplaceRange = (() => {
                    if (selection.rangeCount === 0) return true
                    const cur = selection.getRangeAt(0)
                    return cur.startContainer !== rangeToUse.startContainer ||
                        cur.startOffset !== rangeToUse.startOffset ||
                        cur.endContainer !== rangeToUse.endContainer ||
                        cur.endOffset !== rangeToUse.endOffset
                })()
                if (needReplaceRange) {
                    selection.removeAllRanges()
                    selection.addRange(rangeToUse)
                }

                try {
                    const result = document.execCommand(command, false, value)
                    if (!result) {
                        console.warn(`execCommand ${command} 执行失败`)
                    }
                } catch (e) {
                    console.error(`execCommand ${command} 执行出错:`, e)
                }

                // 分阶段恢复：同步、微任务、两帧后，尽量减少布局抖动带来的闪烁
                // 同步恢复滚动
                editableContainer.scrollTop = prevScrollTop

                Promise.resolve().then(() => {
                    editableContainer.scrollTop = prevScrollTop
                })

                requestAnimationFrame(() => {
                    editableContainer.scrollTop = prevScrollTop
                    requestAnimationFrame(() => {
                        editableContainer.scrollTop = prevScrollTop
                    })
                })

                setTimeout(() => {
                    if (savedRange) {
                        try {
                            const newRange = savedRange.cloneRange()
                            const sameAsCurrent = (() => {
                                if (selection.rangeCount === 0) return false
                                const cur = selection.getRangeAt(0)
                                return cur.startContainer === newRange.startContainer &&
                                    cur.startOffset === newRange.startOffset &&
                                    cur.endContainer === newRange.endContainer &&
                                    cur.endOffset === newRange.endOffset
                            })()
                            if (!sameAsCurrent) {
                                selection.removeAllRanges()
                                selection.addRange(newRange)
                            }
                            if (document.activeElement !== editableContainer) {
                                editableContainer.focus()
                            }
                        } catch (e) {
                            // 如果恢复失败，保持当前选区
                        }
                    }
                    // 恢复滚动位置
                    editableContainer.scrollTop = prevScrollTop
                    // 恢复滚动行为
                    editableContainer.style.scrollBehavior = prevScrollBehavior
                }, 0)
            }
            const getComputedStyleFromElement = (element: HTMLElement) => {
                const computed = window.getComputedStyle(element)
                return {
                    bold: computed.fontWeight === 'bold' || Number(computed.fontWeight) >= 600,
                    italic: computed.fontStyle === 'italic',
                    underline: computed.textDecorationLine.includes('underline'),
                    strikeout: computed.textDecorationLine.includes('line-through'),
                    color: computed.color || '#000000',
                    highlight: (() => {
                        const bgColor = computed.backgroundColor
                        return bgColor && bgColor !== 'rgba(0, 0, 0, 0)' && bgColor !== 'transparent' ? bgColor : null
                    })(),
                    font: computed.fontFamily.split(',')[0].replace(/['"]/g, ''),
                    size: Math.round(parseFloat(computed.fontSize)) || 16
                }
            }

            const getCurrentStyle = (): any => {
                const defaultStyle = {
                    type: null,
                    undo: false,
                    redo: false,
                    painter: false,
                    font: '',
                    size: 0,
                    bold: false,
                    italic: false,
                    underline: false,
                    strikeout: false,
                    color: null,
                    highlight: null,
                    rowFlex: null,
                    rowMargin: 0,
                    dashArray: [],
                    level: null,
                    listType: null,
                    listStyle: null,
                    groupIds: null,
                    textDecoration: null,
                    extension: null
                }

                const selection = window.getSelection()
                if (!selection || selection.rangeCount === 0) {
                    return defaultStyle
                }

                const range = selection.getRangeAt(0)
                if (!editableContainer.contains(range.commonAncestorContainer)) {
                    return defaultStyle
                }

                const style: any = {...defaultStyle}

                const getElementStyle = (node: Node): HTMLElement | null => {
                    if (node.nodeType === 3) {
                        return node.parentElement
                    }
                    return node as HTMLElement
                }

                if (range.collapsed) {
                    const element = getElementStyle(range.startContainer)
                    if (element && element !== editableContainer) {
                        const computed = getComputedStyleFromElement(element)
                        Object.assign(style, computed)
                    }
                } else {
                    const startElement = getElementStyle(range.startContainer)
                    const endElement = getElementStyle(range.endContainer)

                    if (startElement && endElement && startElement !== editableContainer && endElement !== editableContainer) {
                        const startStyle = getComputedStyleFromElement(startElement)
                        const endStyle = getComputedStyleFromElement(endElement)

                        style.bold = startStyle.bold && endStyle.bold
                        style.italic = startStyle.italic && endStyle.italic
                        style.underline = startStyle.underline && endStyle.underline
                        style.strikeout = startStyle.strikeout && endStyle.strikeout

                        if (startStyle.color === endStyle.color) {
                            style.color = startStyle.color
                        } else {
                            style.color = startStyle.color
                        }

                        if (startStyle.highlight === endStyle.highlight) {
                            style.highlight = startStyle.highlight
                        } else {
                            style.highlight = startStyle.highlight
                        }

                        if (startStyle.font === endStyle.font) {
                            style.font = startStyle.font
                        } else {
                            style.font = startStyle.font || ''
                        }

                        if (startStyle.size === endStyle.size) {
                            style.size = startStyle.size
                        } else {
                            style.size = startStyle.size || 0
                        }
                    } else if (startElement && startElement !== editableContainer) {
                        const computed = getComputedStyleFromElement(startElement)
                        Object.assign(style, computed)
                    }
                }

                if (!style.color || style.color === 'rgba(0, 0, 0, 0)') {
                    style.color = '#000000'
                }

                if (!style.font) {
                    style.font = 'Microsoft YaHei'
                }

                if (!style.size || style.size === 0) {
                    style.size = 16
                }

                return style
            }

            const triggerStyleChange = (style: any) => {
                const editor = getEditorInstance()
                if (editor) {
                    if (editor.listener?.rangeStyleChange) {
                        editor.listener.rangeStyleChange(style)
                    }
                    if (editor.eventBus?.isSubscribe('rangeStyleChange')) {
                        editor.eventBus.emit('rangeStyleChange', style)
                    }
                }
                if (editorListener?.rangeStyleChange) {
                    editorListener.rangeStyleChange(style)
                }
                if (editorEventBus?.isSubscribe('rangeStyleChange')) {
                    editorEventBus.emit('rangeStyleChange', style)
                }
            }

            originalCommand.executeBold = () => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('bold')
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeBold) {
                    originalMethods.executeBold.call(originalCommand)
                }
            }

            originalCommand.executeItalic = () => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('italic')
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeItalic) {
                    originalMethods.executeItalic.call(originalCommand)
                }
            }

            originalCommand.executeUnderline = (textDecoration?: any) => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('underline')
                    if (textDecoration?.style) {
                        setTimeout(() => {
                            const selection = window.getSelection()
                            if (selection && selection.rangeCount > 0) {
                                const range = selection.getRangeAt(0)
                                if (editableContainer.contains(range.commonAncestorContainer)) {
                                    const selectedElements = range.commonAncestorContainer.nodeType === 3
                                        ? [range.commonAncestorContainer.parentElement]
                                        : [range.commonAncestorContainer as HTMLElement]

                                    selectedElements.forEach(el => {
                                        if (el) {
                                            el.style.textDecorationLine = 'underline'
                                            el.style.textDecorationStyle = textDecoration.style
                                        }
                                    })
                                }
                            }
                        }, 10)
                    }
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeUnderline) {
                    originalMethods.executeUnderline.call(originalCommand, textDecoration)
                }
            }

            originalCommand.executeStrikeout = () => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('strikeThrough')
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeStrikeout) {
                    originalMethods.executeStrikeout.call(originalCommand)
                }
            }

            originalCommand.executeColor = (color: string) => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('foreColor', color)
                    setTimeout(() => {
                        const style = getCurrentStyle()
                        style.color = color
                        triggerStyleChange(style)
                    }, 100)
                } else if (originalMethods.executeColor) {
                    originalMethods.executeColor.call(originalCommand, color)
                }
            }

            originalCommand.executeHighlight = (color: string) => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('backColor', color)
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeHighlight) {
                    originalMethods.executeHighlight.call(originalCommand, color)
                }
            }

            originalCommand.executeFont = (font: string) => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('fontName', font)
                    setTimeout(() => {
                        const style = getCurrentStyle()
                        style.font = font
                        triggerStyleChange(style)
                    }, 100)
                } else if (originalMethods.executeFont) {
                    originalMethods.executeFont.call(originalCommand, font)
                }
            }

            originalCommand.executeSize = (size: number) => {
                if (savedRange || isSelectionInEditable()) {
                    // 统一步进使用固定 px 序列，保持“加/减”可逆
                    const sizeSteps = [8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 32, 36, 40, 48, 56, 72]
                    // 将 px 映射到 execCommand 的 1-7 桶（尽量贴近视觉效果）
                    const mapPxToBucket = (px: number): string => {
                        if (px <= 8) return '2'      // ≈ 10px
                        if (px <= 11) return '2'     // 8-11
                        if (px <= 13) return '3'     // 12-13
                        if (px <= 17) return '4'     // 14-17
                        if (px <= 21) return '5'     // 18-21
                        if (px <= 27) return '6'     // 22-27
                        return '7'                   // 28+
                    }
                    const fontSize = mapPxToBucket(size)
                    execCommand('fontSize', fontSize)
                    setTimeout(() => {
                        const style = getCurrentStyle()
                        // 将当前样式规范化为固定序列中的最近值，方便后续加减可逆
                        const normalized = sizeSteps.reduce((prev, curr) => {
                            return Math.abs(curr - size) < Math.abs(prev - size) ? curr : prev
                        }, 16)
                        style.size = normalized
                        triggerStyleChange(style)
                    }, 100)
                } else if (originalMethods.executeSize) {
                    originalMethods.executeSize.call(originalCommand, size)
                }
            }

            originalCommand.executeSizeAdd = () => {
                if (savedRange || isSelectionInEditable()) {
                    const style = getCurrentStyle()
                    const sizeSteps = [8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 32, 36, 40, 48, 56, 72]
                    const currentSize = style.size || 16
                    // 寻找“>= 当前值”的索引，然后前进一格；确保加后可通过减回到原值
                    const idxGE = sizeSteps.findIndex(s => s >= currentSize)
                    const baseIdx = idxGE === -1 ? sizeSteps.indexOf(16) : idxGE
                    const nextIdx = Math.min(baseIdx + 1, sizeSteps.length - 1)
                    const nextSize = sizeSteps[nextIdx]
                    originalCommand.executeSize(nextSize)
                } else if (originalMethods.executeSizeAdd) {
                    originalMethods.executeSizeAdd.call(originalCommand)
                }
            }

            originalCommand.executeSizeMinus = () => {
                if (savedRange || isSelectionInEditable()) {
                    const style = getCurrentStyle()
                    const sizeSteps = [8, 9, 10, 11, 12, 14, 16, 18, 20, 22, 24, 26, 28, 32, 36, 40, 48, 56, 72]
                    const currentSize = style.size || 16
                    // 寻找“<= 当前值”的最大索引，然后后退一格；确保减后可通过加回到原值
                    const idxLE = (() => {
                        let i = -1
                        for (let k = 0; k < sizeSteps.length; k++) {
                            if (sizeSteps[k] <= currentSize) i = k
                        }
                        return i
                    })()
                    const baseIdx = idxLE === -1 ? sizeSteps.indexOf(16) : idxLE
                    const prevIdx = Math.max(baseIdx - 1, 0)
                    const prevSize = sizeSteps[prevIdx]
                    originalCommand.executeSize(prevSize)
                } else if (originalMethods.executeSizeMinus) {
                    originalMethods.executeSizeMinus.call(originalCommand)
                }
            }

            // 撤销/重做：在可编辑区域内使用浏览器命令，否则回退到原有实现
            originalCommand.executeUndo = () => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('undo')
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeUndo) {
                    originalMethods.executeUndo.call(originalCommand)
                }
            }

            originalCommand.executeRedo = () => {
                if (savedRange || isSelectionInEditable()) {
                    execCommand('redo')
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 50)
                } else if (originalMethods.executeRedo) {
                    originalMethods.executeRedo.call(originalCommand)
                }
            }

            originalCommand.setRangeStyle = () => {
                if (isSelectionInEditable()) {
                    const style = getCurrentStyle()
                    triggerStyleChange(style)
                    return style
                } else if (originalMethods.setRangeStyle) {
                    return originalMethods.setRangeStyle.call(originalCommand)
                }
                return null
            }

            const handleSelectionChange = () => {
                if (isSelectionInEditable()) {
                    saveSelection()
                    setTimeout(() => {
                        triggerStyleChange(getCurrentStyle())
                    }, 10)
                }
            }

            editableContainer.addEventListener('click', () => {
                editableContainer.focus()
                setTimeout(handleSelectionChange, 10)
            })

            editableContainer.addEventListener('mouseup', () => {
                setTimeout(handleSelectionChange, 10)
            })

            editableContainer.addEventListener('keyup', handleSelectionChange)
            editableContainer.addEventListener('keydown', () => {
                if (!editableContainer.contains(document.activeElement)) {
                    editableContainer.focus()
                }
            })

            document.addEventListener('selectionchange', () => {
                if (isSelectionInEditable()) {
                    saveSelection()
                    handleSelectionChange()
                }
            })

            document.addEventListener('mousedown', (e) => {
                const target = e.target as HTMLElement
                if (target && (
                    target.closest('.menu, .editor-toolbar, [class*="toolbar"], [class*="menu-item"]') ||
                    target.closest('[id^="toolbar-"]')
                )) {
                    saveSelection()
                }
            }, true)

            document.addEventListener('click', (e) => {
                const target = e.target as HTMLElement
                if (target && (
                    target.closest('.menu, .editor-toolbar, [class*="toolbar"], [class*="menu-item"]') ||
                    target.closest('[id^="toolbar-"]')
                )) {
                    setTimeout(() => {
                        if (savedRange) {
                            restoreSelection()
                        }
                    }, 10)
                }
            }, true)

            setTimeout(() => {
                editableContainer.focus()
                handleSelectionChange()
            }, 200)

        } catch (error) {
            console.error('docxjs 渲染失败:', error)
            if (scrollParent && originalOverflow) {
                scrollParent.style.overflow = originalOverflow
                scrollParent.removeAttribute('data-original-overflow')
            }
            canvasElements.forEach((el: Element) => {
                ;(el as HTMLElement).style.display = ''
            })
            if (editableContainer.parentNode) {
                editorContainer.removeChild(editableContainer)
            }
            if (styleContainer.parentNode) {
                editorContainer.removeChild(styleContainer)
            }
            throw error
        }
    }
}

