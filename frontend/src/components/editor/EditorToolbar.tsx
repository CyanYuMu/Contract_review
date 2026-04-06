"use client";

import React, {useEffect, useRef, useState} from "react";
import {Spin} from 'antd'
import "./EditorToolbar.css";
import {Dialog} from "@/lib/canvas-editor/components/dialog/Dialog";
import classNames from "classnames";

type Props = {
    editor: any | null;
    onSave?: () => Promise<void>;
};

const FONT_FAMILIES = [
    {name: "微软雅黑", value: "Microsoft YaHei"},
    {name: "华文宋体", value: "华文宋体"},
    {name: "华文黑体", value: "华文黑体"},
    {name: "华文仿宋", value: "华文仿宋"},
    {name: "华文楷体", value: "华文楷体"},
    {name: "华文琥珀", value: "华文琥珀"},
    {name: "华文隶书", value: "华文隶书"},
    {name: "华文新魏", value: "华文新魏"},
    {name: "华文行楷", value: "华文行楷"},
    {name: "华文中宋", value: "华文中宋"},
    {name: "华文彩云", value: "华文彩云"},
    {name: "Arial", value: "Arial"},
    {name: "Segoe UI", value: "Segoe UI"},
    {name: "Ink Free", value: "Ink Free"},
    {name: "Fantasy", value: "Fantasy"},
];

const FONT_SIZES = [
    {name: "初号", value: 56},
    {name: "小初", value: 48},
    {name: "一号", value: 34},
    {name: "小一", value: 32},
    {name: "二号", value: 29},
    {name: "小二", value: 24},
    {name: "三号", value: 21},
    {name: "小三", value: 20},
    {name: "四号", value: 18},
    {name: "小四", value: 16},
    {name: "五号", value: 14},
    {name: "小五", value: 12},
    {name: "六号", value: 10},
    {name: "小六", value: 8},
    {name: "七号", value: 7},
    {name: "八号", value: 6},
];

export default function EditorToolbar({editor, onSave}: Props) {
    const imageInputRef = useRef<HTMLInputElement>(null);
    const menuButtonClickedRef = useRef(false);

    const command = editor?.command;
    const exec = (fn?: () => void) => {
        try {
            fn && fn();
        } catch {
        }
    };

    const [currentFont, setCurrentFont] = useState<string>("字体");
    const [currentSize, setCurrentSize] = useState<string>("字号");
    const [currentColor, setCurrentColor] = useState<string>("#000000");
    const [currentHighlight, setCurrentHighlight] = useState<string>("#ffff00");
    const [underlineVisible, setUnderlineVisible] = useState(false);
    const [listVisible, setListVisible] = useState(false);
    const [watermarkVisible, setWatermarkVisible] = useState(false);
    const [saving, setSaving] = useState<boolean>(false);

    const handleSave = async () => {
        if (saving) return

        try {
            setSaving(true)
            await onSave?.()
        } finally {
            setSaving(false)
        }
    }

    const onFontChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        const value = e.target.value;
        if (!value) return;
        exec(() => command.executeFont(value));
    };

    const onSizeChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        const v = Number(e.target.value);
        if (!v) return;
        exec(() => command.executeSize(v));
    };

    const onColorChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const newColor = e.target.value;
        setCurrentColor(newColor);
        exec(() => command.executeColor(newColor));
        setTimeout(() => {
            if (command && typeof command.setRangeStyle === "function") {
                command.setRangeStyle();
            }
        }, 100);
    };

    const onHighlightChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const newHighlight = e.target.value;
        setCurrentHighlight(newHighlight);
        exec(() => command.executeHighlight(newHighlight));
        setTimeout(() => {
            if (command && typeof command.setRangeStyle === "function") {
                command.setRangeStyle();
            }
        }, 100);
    };

    const onTitleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
        const raw = e.target.value;
        exec(() => command.executeTitle(raw ? Number(raw) : null));
    };

    const onInsertTable = (rows = 3, cols = 3) => {
        exec(() => command.executeInsertTable(rows, cols));
    };

    const onPickImage = () => imageInputRef.current?.click();
    const onPickImageChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;
        const reader = new FileReader();
        reader.readAsDataURL(file);
        reader.onload = () => {
            const img = new Image();
            img.src = reader.result as string;
            img.onload = () => {
                exec(() =>
                    command.executeImage({
                        value: reader.result as string,
                        width: img.width,
                        height: img.height,
                    })
                );
            };
        };
        e.target.value = "";
    };

    const [fontVisible, setFontVisible] = useState(false);
    const [sizeVisible, setSizeVisible] = useState(false);
    const [titleVisible, setTitleVisible] = useState(false);

    useEffect(() => {
        if (!editor?.listener || !command) return;

        const originalHandler = editor.listener.rangeStyleChange;

        const handleRangeStyleChange = (payload: any) => {
            if (originalHandler) {
                originalHandler(payload);
            }

            if (payload.font) {
                setCurrentFont(payload.font);
            }
            if (payload.size) {
                const sizeItem = FONT_SIZES.find((s) => s.value === payload.size);
                setCurrentSize(sizeItem ? sizeItem.name : payload.size.toString());
            }
            if ("color" in payload) {
                if (
                    payload.color !== null &&
                    payload.color !== undefined &&
                    payload.color !== ""
                ) {
                    setCurrentColor(payload.color);
                    setTimeout(() => {
                        const colorInput = document.getElementById(
                            "toolbar-color"
                        ) as HTMLInputElement;
                        if (colorInput) {
                            colorInput.value = payload.color;
                        }
                    }, 0);
                } else {
                    setCurrentColor("#000000");
                    setTimeout(() => {
                        const colorInput = document.getElementById(
                            "toolbar-color"
                        ) as HTMLInputElement;
                        if (colorInput) {
                            colorInput.value = "#000000";
                        }
                    }, 0);
                }
            }
            if ("highlight" in payload) {
                if (
                    payload.highlight !== null &&
                    payload.highlight !== undefined &&
                    payload.highlight !== ""
                ) {
                    setCurrentHighlight(payload.highlight);
                    setTimeout(() => {
                        const highlightInput = document.getElementById(
                            "toolbar-highlight"
                        ) as HTMLInputElement;
                        if (highlightInput) {
                            highlightInput.value = payload.highlight;
                        }
                    }, 0);
                } else {
                    setCurrentHighlight("#ffff00");
                    setTimeout(() => {
                        const highlightInput = document.getElementById(
                            "toolbar-highlight"
                        ) as HTMLInputElement;
                        if (highlightInput) {
                            highlightInput.value = "#ffff00";
                        }
                    }, 0);
                }
            }
        };

        editor.listener.rangeStyleChange = handleRangeStyleChange;

        const initStyle = () => {
            try {
                if (command && typeof command.setRangeStyle === "function") {
                    command.setRangeStyle();
                }
            } catch (e) {
            }
        };

        const initTimer = setTimeout(initStyle, 200);

        const editorContainer = command.getContainer?.();
        const handleEditorInteraction = () => {
            setTimeout(() => {
                try {
                    if (command && typeof command.setRangeStyle === "function") {
                        command.setRangeStyle();
                    }
                } catch (e) {
                }
            }, 50);
        };

        if (editorContainer) {
            editorContainer.addEventListener("click", handleEditorInteraction);
            editorContainer.addEventListener("keyup", handleEditorInteraction);
            editorContainer.addEventListener(
                "selectionchange",
                handleEditorInteraction
            );
        }

        return () => {
            clearTimeout(initTimer);
            if (editor?.listener) {
                editor.listener.rangeStyleChange = originalHandler;
            }
            if (editorContainer) {
                editorContainer.removeEventListener("click", handleEditorInteraction);
                editorContainer.removeEventListener("keyup", handleEditorInteraction);
                editorContainer.removeEventListener(
                    "selectionchange",
                    handleEditorInteraction
                );
            }
        };
    }, [editor, command]);

    const handleStrikeout = () => exec(command?.executeStrikeout);
    const handleSuperscript = () => exec(command?.executeSuperscript);
    const handleSubscript = () => exec(command?.executeSubscript);
    const handleAlignment = () =>
        exec(() => command.executeRowFlex("alignment" as any));
    const handleJustify = () =>
        exec(() => command.executeRowFlex("justify" as any));
    const handleFormatClear = () => exec(command?.executeFormat);
    const handlePainter = (isDbl = false) =>
        exec(() => command.executePainter({isDblclick: isDbl}));
    const handlePageBreak = () => exec(command?.executePageBreak);
    const handlePrint = () => exec(command?.executePrint);

    const [rowMarginVisible, setRowMarginVisible] = useState(false);
    const onRowMarginSelect = (value: number) =>
        exec(() => command.executeRowMargin(value));

    const [separatorVisible, setSeparatorVisible] = useState(false);
    const onSeparatorSelect = (dash: number[]) =>
        exec(() => command.executeSeparator(dash));

    const [searchVisible, setSearchVisible] = useState(false);
    const searchInputRef = useRef<HTMLInputElement>(null);
    const replaceInputRef = useRef<HTMLInputElement>(null);

    const handleCodeBlock = () => {
        const code = window.prompt("请输入代码");
        if (!code) return;
        exec(() =>
            command.executeInsertElementList([
                {value: "\n"},
                ...code.split("").map((c) => ({value: c})),
            ])
        );
    };

    const handleInsertMarkdown = () => {
        const markdown = window.prompt("请输入 Markdown 文本");
        if (!markdown) return;
        if (
            command &&
            typeof (command as any).executeInsertMarkdown === "function"
        ) {
            exec(() => (command as any).executeInsertMarkdown(markdown));
        } else {
            alert("请确保已加载 Markdown 插件");
        }
    };

    const handleLatex = () => {
        const value = window.prompt("请输入LaTeX文本");
        if (!value) return;
        exec(() =>
            command.executeInsertElementList([{type: "latex" as any, value}])
        );
    };

    const handleSearchClick = (e: React.MouseEvent) => {
        const target = e.target as Element;
        if (target.closest(".menu-item__search__collapse")) return;
        setSearchVisible((v) => {
            const next = !v;
            if (next) {
                setTimeout(() => searchInputRef.current?.focus(), 100);
            }
            return next;
        });
    };

    const onSearchInput = (e: React.ChangeEvent<HTMLInputElement>) => {
        exec(() => command.executeSearch(e.target.value || null));
    };

    const onReplace = () => {
        const searchValue = searchInputRef.current?.value;
        const replaceValue = replaceInputRef.current?.value || "";

        if (!searchValue) {
            return;
        }

        exec(() => {
            const navigateInfo = command.getSearchNavigateInfo();
            if (
                navigateInfo &&
                navigateInfo.index !== undefined &&
                navigateInfo.index > 0
            ) {
                command.executeReplace(replaceValue, {index: navigateInfo.index - 1});
            } else {
                command.executeReplace(replaceValue);
            }
        });
    };

    const onHyperlink = () => {
        const name = window.prompt("显示文本");
        if (!name) return;
        const url = window.prompt("链接地址");
        if (!url) return;
        exec(() =>
            command.executeHyperlink({
                url,
                valueList: [{value: name, size: 16}],
            } as any)
        );
    };

    useEffect(() => {
        const handleClickOutside = (evt: MouseEvent) => {
            if (menuButtonClickedRef.current) {
                menuButtonClickedRef.current = false;
                return;
            }

            const target = evt.target as Element;

            const clickedOptions = target.closest(".options");
            if (clickedOptions) {
                return;
            }

            const clickedSearchMenu = target.closest(
                ".menu-item__search, .menu-item__search__collapse"
            );
            if (clickedSearchMenu) {
                return;
            }

            const clickedMenuItem = target.closest(
                ".menu-item__font, .menu-item__size, .menu-item__title, .menu-item__row-margin, .menu-item__separator, .menu-item__underline, .menu-item__list, .menu-item__watermark"
            );
            if (clickedMenuItem) {
                return;
            }

            setFontVisible(false);
            setSizeVisible(false);
            setTitleVisible(false);
            setRowMarginVisible(false);
            setSeparatorVisible(false);
            setUnderlineVisible(false);
            setListVisible(false);
            setWatermarkVisible(false);
        };

        window.addEventListener("click", handleClickOutside, {capture: false});
        // 捕获阶段拦截：点击搜索面板内部，阻断所有后续监听
        const captureGuard = (e: Event) => {
            const target = e.target as Element | null;
            if (!target) return;
            if (target.closest && target.closest(".menu-item__search__collapse")) {
                e.stopPropagation();
            }
        };
        window.addEventListener("mousedown", captureGuard, {capture: true});
        window.addEventListener("click", captureGuard, {capture: true});

        return () => {
            window.removeEventListener("click", handleClickOutside, {
                capture: false,
            });
            window.removeEventListener("mousedown", captureGuard, {
                capture: true,
            } as any);
            window.removeEventListener("click", captureGuard, {
                capture: true,
            } as any);
        };
    }, []);

    return (
        <div className="menu">
            <div className="menu-item">
                <div
                    className="menu-item__undo"
                    title="撤销(Ctrl+Z)"
                    onClick={() => exec(command?.executeUndo)}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__redo"
                    title="重做(Ctrl+Y)"
                    onClick={() => exec(command?.executeRedo)}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__painter"
                    title="格式刷(双击连续)"
                    onClick={() => handlePainter(false)}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__format"
                    title="清除格式"
                    onClick={handleFormatClear}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__font"
                    title="字体"
                    onClick={(e) => {
                        e.stopPropagation();
                        menuButtonClickedRef.current = true;
                        setSizeVisible(false);
                        setTitleVisible(false);
                        setRowMarginVisible(false);
                        setSeparatorVisible(false);
                        setUnderlineVisible(false);
                        setListVisible(false);
                        setFontVisible((prev) => !prev);
                    }}
                >
          <span
              className="select"
              style={{
                  fontFamily: currentFont !== "字体" ? currentFont : undefined,
              }}
          >
            {currentFont !== "字体" ? currentFont : "字体"}
          </span>
                    <div className={`options ${fontVisible ? "visible" : ""}`}>
                        <ul>
                            {FONT_FAMILIES.map((f) => (
                                <li
                                    key={f.value}
                                    data-family={f.value}
                                    style={{fontFamily: f.value}}
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        onFontChange({target: {value: f.value}} as any);
                                        setFontVisible(false);
                                    }}
                                >
                                    {f.name}
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__size"
                    title="设置字号"
                    onClick={(e) => {
                        e.stopPropagation();
                        menuButtonClickedRef.current = true;
                        setFontVisible(false);
                        setTitleVisible(false);
                        setRowMarginVisible(false);
                        setSeparatorVisible(false);
                        setSizeVisible((prev) => !prev);
                    }}
                >
          <span className="select">
            {currentSize !== "字号" ? currentSize : "字号"}
          </span>
                    <div className={`options ${sizeVisible ? "visible" : ""}`}>
                        <ul>
                            {FONT_SIZES.map((s) => (
                                <li
                                    key={s.value}
                                    data-size={s.value}
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        onSizeChange({target: {value: String(s.value)}} as any);
                                        setSizeVisible(false);
                                    }}
                                >
                                    {s.name}
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__size-add"
                    title="增大字号(Ctrl+[)"
                    onClick={() => {
                        exec(command?.executeSizeAdd);
                        setTimeout(() => {
                            if (command && typeof command.setRangeStyle === "function") {
                                command.setRangeStyle();
                            }
                        }, 80);
                    }}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__size-minus"
                    title="减小字号(Ctrl+])"
                    onClick={() => {
                        exec(command?.executeSizeMinus);
                        setTimeout(() => {
                            if (command && typeof command.setRangeStyle === "function") {
                                command.setRangeStyle();
                            }
                        }, 80);
                    }}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__bold"
                    title="加粗(Ctrl+B)"
                    onClick={() => exec(command?.executeBold)}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__italic"
                    title="斜体(Ctrl+I)"
                    onClick={() => exec(command?.executeItalic)}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__underline"
                    title="下划线(Ctrl+U)"
                    onClick={() => {
                        setFontVisible(false);
                        setSizeVisible(false);
                        setTitleVisible(false);
                        setRowMarginVisible(false);
                        setSeparatorVisible(false);
                        setUnderlineVisible(false);
                        setListVisible(false);
                        setUnderlineVisible(!underlineVisible);
                    }}
                >
                    <i></i>
                    <span className="select"></span>
                    <div className={`options ${underlineVisible ? "visible" : ""}`}>
                        <ul>
                            <li
                                data-decoration-style="solid"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command?.executeUnderline({style: "solid" as any})
                                    );
                                    setUnderlineVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                data-decoration-style="double"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command?.executeUnderline({style: "double" as any})
                                    );
                                    setUnderlineVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                data-decoration-style="dashed"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command?.executeUnderline({style: "dashed" as any})
                                    );
                                    setUnderlineVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                data-decoration-style="dotted"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command?.executeUnderline({style: "dotted" as any})
                                    );
                                    setUnderlineVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                data-decoration-style="wavy"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command?.executeUnderline({style: "wavy" as any})
                                    );
                                    setUnderlineVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__strikeout"
                    title="删除线(Ctrl+Shift+X)"
                    onClick={handleStrikeout}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__superscript"
                    title="上标(Ctrl+Shift+ , )"
                    onClick={handleSuperscript}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__subscript"
                    title="下标(Ctrl+Shift+ . )"
                    onClick={handleSubscript}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__color flex flex-col relative items-center justify-center"
                    title="字体颜色"
                    onClick={() => document.getElementById("toolbar-color")?.click()}
                >
                    <i></i>
                    <span
                        className="absolute w-3.5 h-0.5 bottom-0.5 left-1/2 -translate-x-1/2 z-[1] pointer-events-none rounded-sm"
                        style={{backgroundColor: currentColor}}
                    ></span>
                    <input
                        type="color"
                        id="toolbar-color"
                        className="absolute w-px h-px invisible opacity-0"
                        value={currentColor}
                        onChange={onColorChange}
                    />
                </div>
                <div
                    className="menu-item__highlight flex flex-col relative items-center justify-center"
                    title="高亮"
                    onClick={() => document.getElementById("toolbar-highlight")?.click()}
                >
                    <i></i>
                    <span
                        className="absolute w-3.5 h-0.5 bottom-0.5 left-1/2 -translate-x-1/2 z-[1] pointer-events-none rounded-sm"
                        style={{backgroundColor: currentHighlight}}
                    ></span>
                    <input
                        type="color"
                        id="toolbar-highlight"
                        className="absolute w-px h-px invisible opacity-0"
                        value={currentHighlight}
                        onChange={onHighlightChange}
                    />
                </div>
                <div
                    className="menu-item__title"
                    title="切换标题"
                    onClick={() => {
                        setFontVisible(false);
                        setSizeVisible(false);
                        setRowMarginVisible(false);
                        setSeparatorVisible(false);
                        setTitleVisible(!titleVisible);
                    }}
                >
                    <i></i>
                    <span className="select">正文</span>
                    <div className={`options ${titleVisible ? "visible" : ""}`}>
                        <ul>
                            <li
                                style={{fontSize: "16px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: ""}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                正文
                            </li>
                            <li
                                data-level="first"
                                style={{fontSize: "26px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: "1"}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                标题1
                            </li>
                            <li
                                data-level="second"
                                style={{fontSize: "24px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: "2"}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                标题2
                            </li>
                            <li
                                data-level="third"
                                style={{fontSize: "22px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: "3"}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                标题3
                            </li>
                            <li
                                data-level="fourth"
                                style={{fontSize: "20px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: "4"}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                标题4
                            </li>
                            <li
                                data-level="fifth"
                                style={{fontSize: "18px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: "5"}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                标题5
                            </li>
                            <li
                                data-level="sixth"
                                style={{fontSize: "16px"}}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    onTitleChange({target: {value: "6"}} as any);
                                    setTitleVisible(false);
                                }}
                            >
                                标题6
                            </li>
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__left"
                    title="左对齐(Ctrl+L)"
                    onClick={() => exec(() => command.executeRowFlex("left" as any))}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__center"
                    title="居中(Ctrl+E)"
                    onClick={() => exec(() => command.executeRowFlex("center" as any))}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__right"
                    title="右对齐(Ctrl+R)"
                    onClick={() => exec(() => command.executeRowFlex("right" as any))}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__alignment"
                    title="两端对齐(Ctrl+J)"
                    onClick={handleAlignment}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__row-margin"
                    title="行间距"
                    onClick={() => {
                        setFontVisible(false);
                        setSizeVisible(false);
                        setTitleVisible(false);
                        setSeparatorVisible(false);
                        setRowMarginVisible(!rowMarginVisible);
                    }}
                >
                    <i title="行间距"></i>
                    <div className={`options ${rowMarginVisible ? "visible" : ""}`}>
                        <ul>
                            {[1, 1.25, 1.5, 1.75, 2, 2.5, 3].map((v) => (
                                <li
                                    key={v}
                                    data-rowmargin={v}
                                    onMouseDown={(e) => {
                                        e.stopPropagation();
                                        onRowMarginSelect(Number(v));
                                        setRowMarginVisible(false);
                                    }}
                                >
                                    {v}
                                </li>
                            ))}
                        </ul>
                    </div>
                </div>
            </div>
            <div className="menu-item">
                <div
                    className="menu-item__list"
                    title="列表"
                    onClick={() => {
                        setFontVisible(false);
                        setSizeVisible(false);
                        setTitleVisible(false);
                        setRowMarginVisible(false);
                        setSeparatorVisible(false);
                        setUnderlineVisible(false);
                        setListVisible(!listVisible);
                    }}
                >
                    <i></i>
                    <div className={`options ${listVisible ? "visible" : ""}`}>
                        <ul>
                            <li
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() => command.executeList(null, null));
                                    setListVisible(false);
                                }}
                            >
                                <label>取消列表</label>
                            </li>
                            <li
                                data-list-type="ol"
                                data-list-style="decimal"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command.executeList("ol" as any, "decimal" as any)
                                    );
                                    setListVisible(false);
                                }}
                            >
                                <label>有序列表：</label>
                                <ol>
                                    <li>________</li>
                                </ol>
                            </li>
                            <li
                                data-list-type="ul"
                                data-list-style="checkbox"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() =>
                                        command.executeList("ul" as any, "checkbox" as any)
                                    );
                                    setListVisible(false);
                                }}
                            >
                                <label>复选框列表：</label>
                                <ul style={{listStyleType: "☑️ "}}>
                                    <li>________</li>
                                </ul>
                            </li>
                            <li
                                data-list-type="ul"
                                data-list-style="disc"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() => command.executeList("ul" as any, "disc" as any));
                                    setListVisible(false);
                                }}
                            >
                                <label>实心圆点列表：</label>
                                <ul style={{listStyleType: "disc"}}>
                                    <li>________</li>
                                </ul>
                            </li>
                            <li
                                data-list-type="ul"
                                data-list-style="circle"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() => command.executeList("ul" as any, "circle" as any));
                                    setListVisible(false);
                                }}
                            >
                                <label>空心圆点列表：</label>
                                <ul style={{listStyleType: "circle"}}>
                                    <li>________</li>
                                </ul>
                            </li>
                            <li
                                data-list-type="ul"
                                data-list-style="square"
                                onClick={(e) => {
                                    e.stopPropagation();
                                    exec(() => command.executeList("ul" as any, "square" as any));
                                    setListVisible(false);
                                }}
                            >
                                <label>空心方块列表：</label>
                                <ul style={{listStyleType: "☐ "}}>
                                    <li>________</li>
                                </ul>
                            </li>
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__table"
                    title="表格"
                    onClick={() => onInsertTable(3, 3)}
                >
                    <i></i>
                </div>
                <div className="menu-item__image" title="图片" onClick={onPickImage}>
                    <i></i>
                    <input
                        ref={imageInputRef}
                        type="file"
                        accept="image/*"
                        onChange={onPickImageChange}
                    />
                </div>
                <div
                    className="menu-item__separator"
                    title="分割线"
                    onClick={() => {
                        setFontVisible(false);
                        setSizeVisible(false);
                        setTitleVisible(false);
                        setRowMarginVisible(false);
                        setUnderlineVisible(false);
                        setListVisible(false);
                        setWatermarkVisible(false);
                        setSeparatorVisible(!separatorVisible);
                    }}
                >
                    <i title="分割线"></i>
                    <div className={`options ${separatorVisible ? "visible" : ""}`}>
                        <ul>
                            <li
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    onSeparatorSelect([]);
                                    setSeparatorVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    onSeparatorSelect([1, 1]);
                                    setSeparatorVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    onSeparatorSelect([3, 1]);
                                    setSeparatorVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    onSeparatorSelect([4, 4]);
                                    setSeparatorVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    onSeparatorSelect([7, 3, 3, 3]);
                                    setSeparatorVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                            <li
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    onSeparatorSelect([6, 2, 2, 2, 2, 2]);
                                    setSeparatorVisible(false);
                                }}
                            >
                                <i></i>
                            </li>
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__watermark"
                    title="水印(添加、删除)"
                    onClick={() => {
                        setFontVisible(false);
                        setSizeVisible(false);
                        setTitleVisible(false);
                        setRowMarginVisible(false);
                        setUnderlineVisible(false);
                        setListVisible(false);
                        setSeparatorVisible(false);
                        setWatermarkVisible(!watermarkVisible);
                    }}
                >
                    <i title="水印"></i>
                    <div className={`options ${watermarkVisible ? "visible" : ""}`}>
                        <ul>
                            <li
                                data-menu="add"
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    setWatermarkVisible(false);
                                    new Dialog({
                                        title: "水印",
                                        data: [
                                            {
                                                type: "text",
                                                label: "内容",
                                                name: "data",
                                                required: true,
                                                placeholder: "请输入内容",
                                            },
                                            {
                                                type: "color",
                                                label: "颜色",
                                                name: "color",
                                                required: true,
                                                value: "#AEB5C0",
                                            },
                                            {
                                                type: "number",
                                                label: "字体大小",
                                                name: "size",
                                                required: true,
                                                value: "120",
                                            },
                                            {
                                                type: "number",
                                                label: "透明度",
                                                name: "opacity",
                                                required: true,
                                                value: "0.3",
                                            },
                                            {
                                                type: "select",
                                                label: "重复",
                                                name: "repeat",
                                                value: "0",
                                                options: [
                                                    {label: "不重复", value: "0"},
                                                    {label: "重复", value: "1"},
                                                ],
                                            },
                                            {
                                                type: "number",
                                                label: "水平间隔",
                                                name: "horizontalGap",
                                                value: "10",
                                            },
                                            {
                                                type: "number",
                                                label: "垂直间隔",
                                                name: "verticalGap",
                                                value: "10",
                                            },
                                        ],
                                        onConfirm: (payload) => {
                                            const watermark: any = {};
                                            payload.forEach((p) => {
                                                watermark[p.name] = p.value;
                                            });
                                            const repeat = watermark.repeat === "1";
                                            exec(() =>
                                                command.executeAddWatermark({
                                                    data: watermark.data,
                                                    color: watermark.color,
                                                    size: Number(watermark.size),
                                                    opacity: Number(watermark.opacity),
                                                    repeat,
                                                    gap:
                                                        repeat &&
                                                        watermark.horizontalGap &&
                                                        watermark.verticalGap
                                                            ? [
                                                                Number(watermark.horizontalGap),
                                                                Number(watermark.verticalGap),
                                                            ]
                                                            : undefined,
                                                })
                                            );
                                        },
                                    });
                                }}
                            >
                                添加水印
                            </li>
                            <li
                                data-menu="delete"
                                onMouseDown={(e) => {
                                    e.stopPropagation();
                                    setWatermarkVisible(false);
                                    exec(() => command.executeDeleteWatermark());
                                }}
                            >
                                删除水印
                            </li>
                        </ul>
                    </div>
                </div>
                <div
                    className="menu-item__page-break"
                    title="分页符"
                    onClick={handlePageBreak}
                >
                    <i></i>
                </div>
                <div className="menu-item__latex" title="LaTeX" onClick={handleLatex}>
                    <i></i>
                </div>
                <div className="menu-item__search" title="搜索与替换(Ctrl+F)">
                    <i onClick={handleSearchClick}></i>
                    {searchVisible && (
                        <div
                            className="menu-item__search__collapse"
                            style={{display: "block"}}
                            onMouseDown={(e) => e.stopPropagation()}
                            onPointerDown={(e) => e.stopPropagation()}
                            onClick={(e) => e.stopPropagation()}
                        >
                            <div className="menu-item__search__collapse__search">
                                <input
                                    ref={searchInputRef}
                                    type="text"
                                    placeholder="搜索"
                                    onInput={onSearchInput}
                                    onFocus={() => setSearchVisible(true)}
                                    onMouseDown={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                    onPointerDown={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                />
                                <span
                                    onClick={() => {
                                        setSearchVisible(false);
                                        exec(() => command.executeSearch(null));
                                    }}
                                >
                  ×
                </span>
                            </div>
                            <div className="menu-item__search__collapse__replace">
                                <input
                                    ref={replaceInputRef}
                                    type="text"
                                    placeholder="替换"
                                    onFocus={() => setSearchVisible(true)}
                                    onMouseDown={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                    onPointerDown={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                />
                                <button
                                    onMouseDown={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                    onPointerDown={(e) => {
                                        e.stopPropagation();
                                        (e as any).nativeEvent?.stopImmediatePropagation?.();
                                    }}
                                    onClick={onReplace}
                                >
                                    替换
                                </button>
                            </div>
                        </div>
                    )}
                </div>
                <div
                    className="page-scale-minus"
                    title="缩小(Ctrl+-)"
                    onClick={() => exec(command?.executePageScaleMinus)}
                >
                    <i></i>
                </div>
                <div
                    className="page-scale-add"
                    title="放大(Ctrl+=)"
                    onClick={() => exec(command?.executePageScaleAdd)}
                >
                    <i></i>
                </div>
                <div
                    className="menu-item__print"
                    title="打印(Ctrl+P)"
                    onClick={handlePrint}
                >
                    <i></i>
                </div>
                {onSave && (
                    <div
                        className={classNames(["menu-item__save", saving ? '!cursor-not-allowed' : void 0])}
                        title="保存文档(Ctrl+S)"
                        onClick={handleSave}
                    >
                        {saving ? <Spin size='small'/> : <i></i>}
                    </div>
                )}
            </div>
        </div>
    );
}
