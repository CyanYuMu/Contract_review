const version = '0.9.118'
import { Draw } from '../draw/Draw'
import { ICatalog } from '../../interface/Catalog'
import { IEditorResult } from '../../interface/Editor'
import { IGetValueOption } from '../../interface/Draw'
import { deepClone } from '../../utils'
import {
  getWordCountSync,
  getCatalogSync,
  getGroupIdsSync,
  getValueSync
} from './workerSync'

// Next.js 兼容：使用同步方式执行 worker 逻辑
export class WorkerManager {
  private draw: Draw

  constructor(draw: Draw) {
    this.draw = draw
  }

  public getWordCount(): Promise<number> {
    const elementList = this.draw.getOriginalMainElementList()
    return Promise.resolve(getWordCountSync(elementList))
  }

  public getCatalog(): Promise<ICatalog | null> {
    const elementList = this.draw.getOriginalMainElementList()
    const positionList = this.draw.getPosition().getOriginalMainPositionList()
    return Promise.resolve(getCatalogSync(elementList, positionList))
  }

  public getGroupIds(): Promise<string[]> {
    const elementList = this.draw.getOriginalMainElementList()
    return Promise.resolve(getGroupIdsSync(elementList))
  }

  public getValue(options?: IGetValueOption): Promise<IEditorResult> {
    const data = this.draw.getOriginValue(options)
    const result = getValueSync(data, options)
    return Promise.resolve({
      version,
      data: result,
      options: deepClone(this.draw.getOptions())
    })
  }
}
