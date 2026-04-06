import Editor from '../../editor'

export default function docxPlugin(editor: Editor) {
  if (typeof window === 'undefined') {
    return
  }
  
  const command = editor.command
  
  if (typeof window !== 'undefined') {
    Promise.all([
      import('./exportDocx'),
      import('./importDocx')
    ]).then(([exportDocxModule, importDocxModule]) => {
      const exportDocx = exportDocxModule.default
      const importDocx = importDocxModule.default
      command.executeExportDocx = exportDocx(command)
      command.executeImportDocx = importDocx(command)
    }).catch(() => {})
  }
}

