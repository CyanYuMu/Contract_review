import React, {useEffect, useState} from 'react';
import {generateMockYesterdayData, signboardMock} from '@/mock/mock';
import {getContractReviewOverview, getRevisionStatistics} from '@/lib/api/signboard';
import {DepartmentUsageItem, OverviewResponse, RevisionsResponse, WordCloudItem} from '@/lib/Interface';
import {Select} from "antd";
import './Signboard.css';
import WordCloudChart from './worldCloud';
import DepartmentRank from './departmentRank';
import RiskTypeRank from './riskTypeRank'
import TrendChart from './trendChart';

interface YesterdayData {
    reviewedCount: number;
    useDeptCount: number;
    usePersonCount: number;
    ServiceCount: number;
    budgetCount: number;
    infrastructrueCount: number;
}

const Signboard: React.FC = () => {
    // const trendChartRef = useRef<HTMLDivElement>(null);
    const [wordCloudData, setWordCloudData] = useState<WordCloudItem[]>([]);
    const [overviewData, setOverviewData] = useState<OverviewResponse | null>(null);
    const [revisionData, setRevisionData] = useState<RevisionsResponse | null>(null);
    const [departmentUsageData, setDepartmentUsageData] = useState<DepartmentUsageItem[]>([]);
    const [overviewLoading, setOverviewLoading] = useState<boolean>(true);
    const [revisionLoading, setRevisionLoading] = useState<boolean>(true);
    const [departmentLoading, setDepartmentLoading] = useState<boolean>(true);
    const [overviewError, setOverviewError] = useState<string>('');
    const [revisionsError, setRevisionsError] = useState<string>('');
    const [departmentError, setDepartmentError] = useState<string>('');
    const [sortField, setSortField] = useState<string>('total');
    const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('desc');
    const [yesterdayData, setYesterdayData] = useState<YesterdayData | null>(null);
    const [yesterdayLoading, setYesterdayLoading] = useState<boolean>(true);
    const [yesterdayError, setYesterdayError] = useState<string>('');

    useEffect(() => {
        // 加载概览数据
        const fetchOverviewData = async () => {
            try {
                setOverviewLoading(true);
                const response = await getContractReviewOverview();
                if (response.code === 200 && response.data) {
                    setOverviewData(response.data);
                } else {
                    setOverviewError(response.msg || '获取数据失败');
                }
            } catch (err) {
                setOverviewError(err instanceof Error ? err.message : '网络异常');
            } finally {
                setOverviewLoading(false);
            }
        };
        fetchOverviewData();

        // 加载修订数据
        const fetchRevisionData = async () => {
            try {
                setRevisionLoading(true);
                const response = await getRevisionStatistics();
                if (response.code === 200 && response.data) {
                    setRevisionData(response.data);
                } else {
                    setRevisionsError(response.msg || '获取数据失败');
                }
            } catch (err) {
                setRevisionsError(err instanceof Error ? err.message : '网络异常');
            } finally {
                setRevisionLoading(false);
            }
        };
        fetchRevisionData();

        // 加载业务单位使用数据
        // const fetchDepartmentUsage = async () => {
        //   try {
        //     setDepartmentLoading(true);
        //     const data = await getDepartmentUsage();
        //     setDepartmentUsageData(data);
        //     setDepartmentError('');
        //   } catch (err) {
        //     setDepartmentError(err instanceof Error ? err.message : '数据加载失败，请重试');
        //   } finally {
        //     setDepartmentLoading(false);
        //   }
        // };
        // fetchDepartmentUsage();
        const fetchYesterdayData = async () => {
            try {
                setYesterdayLoading(true);
                // const response = await getYesterdayContractData();
                // if (response.code === 200 && response.data) {
                //   setYesterdayData(response.data);
                // } else {
                //   setYesterdayError('获取昨日数据失败');
                // }
                const mockYesterdayData = generateMockYesterdayData();
                setYesterdayData(mockYesterdayData);
            } catch (err) {
                setYesterdayError(err instanceof Error ? err.message : '网络异常');
            } finally {
                setYesterdayLoading(false);
            }
        };
        fetchYesterdayData();
        setWordCloudData(signboardMock.wordCloudData);
    }, []);
    const departmentUsagedata: DepartmentUsageItem[] = signboardMock.DepartmentUsageItem.map(item => ({
        department_name: item.department_name,
        contract_review: item.contract_review,
        contract_verification: item.contract_verification,
        contract_comparison: item.contract_comparison,
        total: item.total

    }));
    const calculateChange = (todayValue: number, yesterdayValue: number) => {
        if (yesterdayLoading || yesterdayError || !yesterdayData) return {change: 0, percent: 0};
        const change = todayValue - yesterdayValue; // 增减差值
        return {change};
    };
    const formatNumberWithCommas = (num: number) => {
        return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    };
    const handleSort = (field: string) => {
        if (sortField === field) {
            setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
        } else {
            setSortField(field);
            setSortDirection('desc');
        }
    };
    const sortedData = [...departmentUsagedata].sort((a, b) => {
        const valueA = a[sortField as keyof DepartmentUsageItem];
        const valueB = b[sortField as keyof DepartmentUsageItem];
        if (typeof valueA === 'number' && typeof valueB === 'number') {
            return sortDirection === 'asc' ? valueA - valueB : valueB - valueA;
        }
        if (typeof valueA === 'string' && typeof valueB === 'string') {
            return sortDirection === 'asc'
                ? valueA.localeCompare(valueB)
                : valueB.localeCompare(valueA);
        }
        return 0;
    });

    return (
        <div className="content ">
            <div className='left-content'>
                {/* 顶部卡片区域 */}
                <div className='topCard '>
                    {overviewLoading || overviewError || !overviewData ? (
                        <div className="topCardL  flex flex-nowrap gap-4 bg-white rounded-lg p-4 shadow">
                            <div className='inTopCardL'>审阅合同（个）<p className='stat-number'>-</p></div>
                            <div className='inTopCardL'>服务业务部门（个）<p className='stat-number'>-</p></div>
                            <div className='inTopCardL'>服务师生（人）<p className='stat-number'>-</p></div>
                            <div className='inTopCardL'>累计审阅金额（万元）<p className='stat-number'>-</p></div>
                        </div>
                    ) : (
                        <div className="topCardL  flex flex-nowrap gap-4 bg-white rounded-lg p-4 shadow">
                            <div className='inTopCardL'>审阅合同（个）<p
                                className='stat-number'>{formatNumberWithCommas(overviewData.reviewed_contracts ?? 0)}</p>
                            </div>
                            <div className='inTopCardL'>服务业务部门（个）<p
                                className='stat-number'>{formatNumberWithCommas(overviewData.service_departments)}</p>
                            </div>
                            <div className='inTopCardL'>服务师生（人）<p
                                className='stat-number'>{formatNumberWithCommas(overviewData.served_faculty_students)}</p>
                            </div>
                            <div className='inTopCardL'>累计审阅金额（万元）<p
                                className='stat-number'>{(overviewData.total_reviewed_amount / 10000).toFixed(2)}</p>
                            </div>
                        </div>
                    )}

                    {revisionLoading || revisionsError || !revisionData ? (
                        <div className="topCardR  flex flex-nowrap gap-4 bg-white rounded-lg p-4 shadow">
                            <div className='inTopCardR'>修订风险点（个）<p className='stat-number'>-</p></div>
                            <div className='inTopCardR'>修订错误点（个）<p className='stat-number'>-</p></div>
                        </div>
                    ) : (
                        <div className="topCardR  flex flex-nowrap gap-4 bg-white rounded-lg p-4 shadow">
                            <div className='inTopCardR'>修订风险点（个）<p
                                className='stat-number'>{formatNumberWithCommas(revisionData?.risk_points_revised)}</p>
                            </div>
                            <div className='inTopCardR'>修订错误点（个）<p
                                className='stat-number'>{formatNumberWithCommas(revisionData?.error_points_revised)}</p>
                            </div>
                        </div>
                    )}
                </div>


                {/* 合同审阅数据 */}
                <div className='middleCard text-[black]'>
                    <div className='titleBox'><span className='svg'></span><span
                        className='title'>合同审阅数据</span><span className='describe'>以下为今日数据，实时更新</span>
                    </div>
                    <div className='flex'>
                        <div className='review_data'>
                            审阅合同（份）
                            <p className='stat-number'>{signboardMock.contractReviewData.reviewedCount}</p>
                            {yesterdayLoading || yesterdayError || !yesterdayData ? (
                                <p>-</p>
                            ) : (
                                <>
                                    {(() => {
                                        const {change} = calculateChange(
                                            signboardMock.contractReviewData.reviewedCount,
                                            yesterdayData.reviewedCount
                                        );
                                        return (
                                            <p>
                                                较昨日 {change >= 0 ? <span className='up'></span> :
                                                <span className='down'></span>}
                                                {`${Math.abs(change)} `}
                                            </p>
                                        );
                                    })()}
                                </>
                            )}
                        </div>

                        {/* 使用单位（个） */}
                        <div className='review_data'>
                            使用单位（个）
                            <p className='stat-number'>{signboardMock.contractReviewData.useDeptCount}</p>
                            {yesterdayLoading || yesterdayError || !yesterdayData ? (
                                <p>-</p>
                            ) : (
                                <>
                                    {(() => {
                                        const {change} = calculateChange(
                                            signboardMock.contractReviewData.useDeptCount,
                                            yesterdayData.useDeptCount
                                        );
                                        return (
                                            <p>
                                                较昨日 {change >= 0 ? <span className='up'></span> :
                                                <span className='down'></span>}
                                                {`${Math.abs(change)} `}
                                            </p>
                                        );
                                    })()}
                                </>
                            )}
                        </div>

                        {/* 使用人数（个） */}
                        <div className='review_data'>
                            使用人数（个）
                            <p className='stat-number'>{signboardMock.contractReviewData.usePersonCount}</p>
                            {yesterdayLoading || yesterdayError || !yesterdayData ? (
                                <p>-</p>
                            ) : (
                                <>
                                    {(() => {
                                        const {change} = calculateChange(
                                            signboardMock.contractReviewData.usePersonCount,
                                            yesterdayData.usePersonCount
                                        );
                                        return (
                                            <p>
                                                较昨日 {change >= 0 ? <span className='up'></span> :
                                                <span className='down'></span>}
                                                {`${Math.abs(change)} `}
                                            </p>
                                        );
                                    })()}
                                </>
                            )}
                        </div>

                        {/* 服务类合同（份） */}
                        <div className='review_data'>
                            服务类合同（份）
                            <p className='stat-number'>{signboardMock.contractReviewData.ServiceCount}</p>
                            {yesterdayLoading || yesterdayError || !yesterdayData ? (
                                <p>-</p>
                            ) : (
                                <>
                                    {(() => {
                                        const {change} = calculateChange(
                                            signboardMock.contractReviewData.ServiceCount,
                                            yesterdayData.ServiceCount
                                        );
                                        return (
                                            <p>
                                                较昨日 {change >= 0 ? <span className='up'></span> :
                                                <span className='down'></span>}
                                                {`${Math.abs(change)} `}
                                            </p>
                                        );
                                    })()}
                                </>
                            )}
                        </div>

                        {/* 货物类合同（份） */}
                        <div className='review_data'>
                            货物类合同（份）
                            <p className='stat-number'>{signboardMock.contractReviewData.budgetCount}</p>
                            {yesterdayLoading || yesterdayError || !yesterdayData ? (
                                <p>-</p>
                            ) : (
                                <>
                                    {(() => {
                                        const {change} = calculateChange(
                                            signboardMock.contractReviewData.budgetCount,
                                            yesterdayData.budgetCount
                                        );
                                        return (
                                            <p>
                                                较昨日 {change >= 0 ? <span className='up'></span> :
                                                <span className='down'></span>}
                                                {`${Math.abs(change)} `}
                                            </p>
                                        );
                                    })()}
                                </>
                            )}
                        </div>

                        {/* 基建类合同（份） */}
                        <div className='review_data'>
                            基建类合同（份）
                            <p className='stat-number'>{signboardMock.contractReviewData.infrastructrueCount}</p>
                            {yesterdayLoading || yesterdayError || !yesterdayData ? (
                                <p>-</p>
                            ) : (
                                <>
                                    {(() => {
                                        const {change} = calculateChange(
                                            signboardMock.contractReviewData.infrastructrueCount,
                                            yesterdayData.infrastructrueCount
                                        );
                                        return (
                                            <p>
                                                较昨日 {change >= 0 ? <span className='up'></span> :
                                                <span className='down'></span>}
                                                {`${Math.abs(change)} `}
                                            </p>
                                        );
                                    })()}
                                </>
                            )}
                        </div>
                    </div>
                </div>
                {/* 趋势图 */}
                <div className='trendBox '>
                    <div className='trendChart flex'  style={{width: '49%'}}>
                        <span className='svg'></span>
                        {/* <span className='relative'><select
                            style={{
                                position: 'absolute',
                                top: '30px',

                                marginLeft: '530px',
                                cursor: 'pointer',
                                fontSize: '14px',
                                lineHeight: ' 20px',
                                color: 'rgba(90, 96, 127, 1)',
                            }}
                        >
              <option value="2024">今年</option>
              <option value="2023">去年</option>
              <option value="threeYears">近3年</option>
            </select></span> */}
                        <TrendChart></TrendChart>
                    </div>
                    {/* 词云图 */}
                    <div className='reviewWordCloud' style={{width: '49.5%', marginLeft: '18px'}}>
                        <div className='titleBox '><span className='svg'></span><span
                            className='title'>风险点词云图</span>

                        </div>
                        {/* <Select
                            style={{
                                marginTop: '10px',
                                marginLeft: '33.125rem',
                                cursor: 'pointer',
                                fontSize: '14px',
                                zIndex: '10',
                                lineHeight: ' 20px',
                                color: 'rgba(90, 96, 127, 1)',
                                border: 'none',

                            }}
                            defaultValue='2025'
                        >
                            <Select.Option value="2025">今年</Select.Option>
                            <Select.Option value="2024">去年</Select.Option>
                            <Select.Option value="threeYears">近3年</Select.Option>
                        </Select> */}
                        {wordCloudData.length > 0 && <WordCloudChart data={wordCloudData}/>}
                    </div>
                </div>
            </div>

            <div className='right-content'>
                <div className='rankBox'>
                    <div className='rankBg'>
                        <div className='titleBox '><span className='svg'></span><span
                            className='title'>单位合同审阅排行榜</span></div>
                        <div className='rank'>
                            <DepartmentRank/>
                        </div>
                    </div>
                    <div className='rankBg mt-[30px]'>
                        <div className='titleBox'><span className='svg'></span><span
                            className='title'>风险类型排行榜</span></div>
                        <div className='rank'>
                            <RiskTypeRank/>

                        </div>


                    </div>
                </div>
            </div>
        </div>


    );
};

export default Signboard;