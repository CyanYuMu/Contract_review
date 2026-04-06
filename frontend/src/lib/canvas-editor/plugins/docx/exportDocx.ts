import Color from "color";
import {Command, ElementType, IElement, ListStyle,} from "../../editor";
import {saveAs} from "./utils";

async function convertElementToParagraphChild(
    element: IElement,
    docx: any
): Promise<any> {
    if (element.type === ElementType.IMAGE) {
        const imageData = element.value;
        const isBase64 = imageData.startsWith("data:");
        const imageType = isBase64 ? imageData.split(";")[0].split("/")[1] : "png";

        return new docx.ImageRun({
            data: imageData,
            transformation: {
                width: element.width!,
                height: element.height!,
            },
            type: imageType as "png" | "jpeg" | "gif" | "bmp" | "svg+xml",
            fallback: imageData,
        } as any);
    }
    if (element.type === ElementType.HYPERLINK) {
        return new docx.ExternalHyperlink({
            children: [
                new docx.TextRun({
                    text: element.valueList?.map((child) => child.value).join(""),
                    style: "Hyperlink",
                }),
            ],
            link: element.url!,
        });
    }
    if (element.type === ElementType.TAB) {
        return new docx.TextRun({
            children: [new docx.Tab()],
        });
    }
    if (element.type === ElementType.LATEX) {
        return new docx.MathRun(element.value);
    }
    return new docx.TextRun({
        font: element.font,
        text: element.value,
        bold: element.bold,
        size: `${(element.size || 16) / 0.75}pt`,
        color: Color(element.color).hex() || "#000000",
        italics: element.italic,
        strike: element.strikeout,
        highlight: element.highlight
            ? (Color(element.highlight).hex() as
                | "yellow"
                | "green"
                | "cyan"
                | "magenta"
                | "blue"
                | "red"
                | "darkBlue"
                | "darkCyan"
                | "darkGreen"
                | "darkMagenta"
                | "darkRed"
                | "darkYellow"
                | "lightGray"
                | "darkGray"
                | "black"
                | "white"
                | "none")
            : undefined,
        superScript: element.type === ElementType.SUPERSCRIPT,
        subScript: element.type === ElementType.SUBSCRIPT,
        underline: element.underline ? {} : undefined,
    });
}

async function convertElementListToDocxChildren(
    elementList: IElement[],
    docx: any
): Promise<any[]> {
    const children: any[] = [];

    let paragraphChild: any[] = [];

    async function appendParagraph() {
        if (paragraphChild.length) {
            children.push(
                new docx.Paragraph({
                    children: paragraphChild,
                })
            );
            paragraphChild = [];
        }
    }

    for (let e = 0; e < elementList.length; e++) {
        const element = elementList[e];
        if (element.type === ElementType.TITLE) {
            await appendParagraph();
            const headingLevels = [
                docx.HeadingLevel.HEADING_1,
                docx.HeadingLevel.HEADING_2,
                docx.HeadingLevel.HEADING_3,
                docx.HeadingLevel.HEADING_4,
                docx.HeadingLevel.HEADING_5,
                docx.HeadingLevel.HEADING_6,
            ];
            const levelValue = element.level || 1;
            const levelIndex = typeof levelValue === "number" ? levelValue - 1 : 0;
            const headingLevel =
                headingLevels[levelIndex] || docx.HeadingLevel.HEADING_1;
            const paragraphChildren = await Promise.all(
                element.valueList?.map((child) =>
                    convertElementToParagraphChild(child, docx)
                ) || []
            );
            children.push(
                new docx.Paragraph({
                    heading: headingLevel,
                    children: paragraphChildren,
                })
            );
        } else if (element.type === ElementType.LIST) {
            await appendParagraph();
            const listChildren =
                element.valueList
                    ?.map((item) => item.value)
                    .join("")
                    .split("\n")
                    .map(
                        (text, index) =>
                            new docx.Paragraph({
                                children: [
                                    new docx.TextRun({
                                        text: `${
                                            !element.listStyle ||
                                            element.listStyle === ListStyle.DECIMAL
                                                ? `${index + 1}. `
                                                : `• `
                                        }${text}`,
                                    }),
                                ],
                            })
                    ) || [];
            children.push(...listChildren);
        } else if (element.type === ElementType.TABLE) {
            await appendParagraph();
            const {trList} = element;
            const tableRowList: any[] = [];
            for (let r = 0; r < trList!.length; r++) {
                const tdList = trList![r].tdList;
                const tableCellList: any[] = [];
                for (let c = 0; c < tdList.length; c++) {
                    const td = tdList[c];
                    const cellChildren = await convertElementListToDocxChildren(
                        td.value,
                        docx
                    );
                    tableCellList.push(
                        new docx.TableCell({
                            columnSpan: td.colspan,
                            rowSpan: td.rowspan,
                            children:
                                cellChildren.length > 0
                                    ? cellChildren
                                    : [
                                        new docx.Paragraph({
                                            children: [new docx.TextRun("")],
                                        }),
                                    ],
                            borders: {
                                top: {
                                    style: docx.BorderStyle.SINGLE,
                                    size: 1,
                                    color: "000000",
                                },
                                bottom: {
                                    style: docx.BorderStyle.SINGLE,
                                    size: 1,
                                    color: "000000",
                                },
                                left: {
                                    style: docx.BorderStyle.SINGLE,
                                    size: 1,
                                    color: "000000",
                                },
                                right: {
                                    style: docx.BorderStyle.SINGLE,
                                    size: 1,
                                    color: "000000",
                                },
                            },
                            margins: {
                                top: 100,
                                bottom: 100,
                                left: 100,
                                right: 100,
                            },
                            verticalAlign: docx.VerticalAlign.CENTER,
                        })
                    );
                }
                tableRowList.push(
                    new docx.TableRow({
                        children: tableCellList,
                    })
                );
            }
            children.push(
                new docx.Table({
                    rows: tableRowList,
                    width: {
                        size: 100,
                        type: docx.WidthType.PERCENTAGE,
                    },
                    borders: {
                        top: {style: docx.BorderStyle.SINGLE, size: 1, color: "000000"},
                        bottom: {
                            style: docx.BorderStyle.SINGLE,
                            size: 1,
                            color: "000000",
                        },
                        left: {style: docx.BorderStyle.SINGLE, size: 1, color: "000000"},
                        right: {style: docx.BorderStyle.SINGLE, size: 1, color: "000000"},
                        insideHorizontal: {
                            style: docx.BorderStyle.SINGLE,
                            size: 1,
                            color: "000000",
                        },
                        insideVertical: {
                            style: docx.BorderStyle.SINGLE,
                            size: 1,
                            color: "000000",
                        },
                    },
                })
            );
        } else if (element.type === ElementType.DATE) {
            const dateChildren = await Promise.all(
                element.valueList?.map((child) =>
                    convertElementToParagraphChild(child, docx)
                ) || []
            );
            paragraphChild.push(...dateChildren);
        } else {
            if (/^\n/.test(element.value)) {
                await appendParagraph();
                element.value = element.value.replace(/^\n/, "");
            }
            paragraphChild.push(await convertElementToParagraphChild(element, docx));
        }
    }
    await appendParagraph();
    return children;
}

export interface IExportDocxOption {
    fileName: string;
}

declare module "../../editor" {
    interface Command {
        executeExportDocx(options: IExportDocxOption): Promise<Blob>;
    }
}

export default function (command: Command) {
    return async function (options: IExportDocxOption) {
        return new Promise<Blob>(async (resolve, reject) => {
            if (typeof window === "undefined") {
                reject(new Error("docx 导出功能仅在客户端可用"));
                throw new Error("docx 导出功能仅在客户端可用");
            }

            const {fileName} = options;

            // 检查是否存在 docx-preview 渲染的可编辑容器
            const editableContainer = document.getElementById(
                "docx-editable-container"
            );

            if (editableContainer) {
                // 如果是通过 docx-preview 导入的文档，导出为 HTML 格式（Word 可以打开）
                let htmlContent = editableContainer.innerHTML;

                // 清理 HTML 内容，移除 docx-preview 特有的类和样式
                const tempDiv = document.createElement("div");
                tempDiv.innerHTML = htmlContent;

                // 移除 docx-wrapper 和其他容器，只保留实际内容
                const docxWrapper = tempDiv.querySelector(".docx-wrapper");
                if (docxWrapper) {
                    htmlContent = docxWrapper.innerHTML;
                }

                // 清理内联样式中的复杂属性
                const cleanedDiv = document.createElement("div");
                cleanedDiv.innerHTML = htmlContent;

                // 移除所有 class 属性，保留重要的内联样式
                cleanedDiv.querySelectorAll("*").forEach((el) => {
                    el.removeAttribute("class");
                    // 保留基本样式，移除复杂的样式
                    const style = el.getAttribute("style");
                    if (style) {
                        // 保留重要样式
                        const basicStyles = [];
                        if (style.includes("text-align")) {
                            const match = style.match(/text-align:\s*([^;]+)/);
                            if (match) basicStyles.push(`text-align: ${match[1]}`);
                        }
                        if (style.includes("font-weight")) {
                            const match = style.match(/font-weight:\s*([^;]+)/);
                            if (match) basicStyles.push(`font-weight: ${match[1]}`);
                        }
                        if (style.includes("font-size")) {
                            const match = style.match(/font-size:\s*([^;]+)/);
                            if (match) basicStyles.push(`font-size: ${match[1]}`);
                        }
                        if (style.includes("font-family")) {
                            const match = style.match(/font-family:\s*([^;]+)/);
                            if (match) basicStyles.push(`font-family: ${match[1]}`);
                        }
                        if (style.includes("color")) {
                            const match = style.match(/color:\s*([^;]+)/);
                            if (match) basicStyles.push(`color: ${match[1]}`);
                        }
                        if (style.includes("background-color")) {
                            const match = style.match(/background-color:\s*([^;]+)/);
                            if (match) basicStyles.push(`background-color: ${match[1]}`);
                        }
                        if (style.includes("text-decoration")) {
                            const match = style.match(/text-decoration:\s*([^;]+)/);
                            if (match) basicStyles.push(`text-decoration: ${match[1]}`);
                        }
                        if (style.includes("font-style")) {
                            const match = style.match(/font-style:\s*([^;]+)/);
                            if (match) basicStyles.push(`font-style: ${match[1]}`);
                        }
                        if (basicStyles.length > 0) {
                            el.setAttribute("style", basicStyles.join("; "));
                        } else {
                            el.removeAttribute("style");
                        }
                    }
                });

                htmlContent = cleanedDiv.innerHTML;

                // 导出为 HTML 格式的 .docx 文件（Word 可以打开）
                const fullHtml = `<!DOCTYPE html>
<html xmlns:o='urn:schemas-microsoft-com:office:office' xmlns:w='urn:schemas-microsoft-com:office:word' xmlns='http://www.w3.org/TR/REC-html40'>
<head>
<meta charset='utf-8'>
<title>${fileName}</title>
<!--[if gte mso 9]>
<xml>
<w:WordDocument>
<w:View>Print</w:View>
<w:Zoom>100</w:Zoom>
<w:DoNotOptimizeForBrowser/>
</w:WordDocument>
</xml>
<![endif]-->
<style>
@page Section1 {
  size: 595.3pt 841.9pt;
  margin: 72pt 85.05pt 72pt 85.05pt;
  mso-header-margin: 35.4pt;
  mso-footer-margin: 35.4pt;
  mso-paper-source: 0;
}
div.Section1 { page: Section1; }
body {
  font-family: 'Microsoft YaHei', '宋体', SimSun, Arial;
  font-size: 12pt;
  line-height: 1.5;
}
table {
  border-collapse: collapse;
  width: 100%;
  mso-table-lspace: 0pt;
  mso-table-rspace: 0pt;
}
td, th {
  border: 1pt solid black;
  padding: 5pt;
  mso-border-alt: solid black 0.5pt;
}
p {
  margin: 0;
  mso-pagination: widow-orphan;
}
</style>
</head>
<body>
<div class="Section1">
${htmlContent}
</div>
</body>
</html>`;

                const blob = new Blob(["\ufeff" + fullHtml], {
                    type: "application/msword;charset=utf-8",
                });
                resolve(blob)
                saveAs(blob, `${fileName}.docx`);
            } else {
                const editorValue = command.getValue();

                const {
                    data: {header, main, footer},
                } = editorValue;

                const docx = await import("docx");

                const [headerChildren, footerChildren, mainChildren] = await Promise.all([
                    convertElementListToDocxChildren(header || [], docx),
                    convertElementListToDocxChildren(footer || [], docx),
                    convertElementListToDocxChildren(main || [], docx),
                ]);

                const doc = new docx.Document({
                    sections: [
                        {
                            headers: {
                                default: new docx.Header({
                                    children: headerChildren,
                                }),
                            },
                            footers: {
                                default: new docx.Footer({
                                    children: footerChildren,
                                }),
                            },
                            children: mainChildren,
                        },
                    ],
                });

                docx.Packer.toBlob(doc).then((blob) => {
                    resolve(blob)
                    saveAs(blob, `${fileName}.docx`);
                }).catch((err) => {
                    reject(err)
                });
            }
        })
    };
}