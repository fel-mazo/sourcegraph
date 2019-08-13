import CommentTextMultipleIcon from 'mdi-react/CommentTextMultipleIcon'
import React from 'react'
import { RepositoryIcon } from '../../../../../shared/src/components/icons'
import { ErrorLike, isErrorLike } from '../../../../../shared/src/util/errors'
import { SummaryCountBar, SummaryCountItemDescriptor } from '../../../components/summaryCountBar/SummaryCountBar'
import { DiffStat } from '../../../repo/compare/DiffStat'
import { DiffIcon, GitPullRequestIcon } from '../../../util/octicons'
import { DiagnosticsIcon } from '../../checks/icons'
import { CampaignImpactSummary } from './useCampaignImpactSummary'

const LOADING = 'loading' as const

interface Props {
    impactSummary: typeof LOADING | CampaignImpactSummary | ErrorLike
    baseURL: string
    urlFragmentOrPath: '/' | '#'

    className?: string
}

interface Context extends CampaignImpactSummary, Pick<Props, 'urlFragmentOrPath'> {
    baseURL: string
}

const ITEMS: SummaryCountItemDescriptor<Context>[] = [
    {
        noun: 'discussion',
        icon: CommentTextMultipleIcon,
        count: c => c.discussions,
        condition: c => c.discussions > 0,
        url: c => c.baseURL,
    },
    {
        noun: 'issue',
        icon: DiagnosticsIcon,
        count: c => c.issues,
        condition: c => c.issues > 0,
        url: c => `${c.baseURL}${c.urlFragmentOrPath}threads`,
    },
    {
        noun: 'changeset',
        icon: GitPullRequestIcon,
        count: c => c.changesets,
        condition: c => c.changesets > 0,
        url: c => `${c.baseURL}${c.urlFragmentOrPath}threads`,
    },
    {
        noun: 'diagnostic',
        icon: DiagnosticsIcon,
        count: c => c.diagnostics,
        url: c => `${c.baseURL}${c.urlFragmentOrPath}diagnostics`,
        condition: c => c.discussions > 0,
    },
    {
        noun: 'repository affected',
        pluralNoun: 'repositories affected',
        icon: RepositoryIcon,
        count: c => c.repositories,
        url: c => `${c.baseURL}${c.urlFragmentOrPath}changes`,
    },
    {
        noun: 'file changed',
        pluralNoun: 'files changed',
        icon: DiffIcon,
        count: c => c.files,
        url: c => `${c.baseURL}${c.urlFragmentOrPath}changes`,
        after: c => <DiffStat {...c.diffStat} expandedCounts={true} className="d-inline-flex ml-3" />,
    },
]

/**
 * A bar that summarizes the contents and impact of a campaign.
 */
export const CampaignImpactSummaryBar: React.FunctionComponent<Props> = ({
    impactSummary,
    baseURL,
    urlFragmentOrPath,
    className = '',
}) =>
    impactSummary !== LOADING && !isErrorLike(impactSummary) ? (
        <SummaryCountBar<Context>
            className={className}
            context={{ ...impactSummary, baseURL, urlFragmentOrPath }}
            itemDescriptors={ITEMS}
        />
    ) : null
