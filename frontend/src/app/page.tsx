import {Suspense} from "react";
import HomeClient from "./homepage";

export default function Page() {
    return (
        <Suspense fallback={<div className="p-6">加载中...</div>}>
            <HomeClient/>
        </Suspense>
    );
}
